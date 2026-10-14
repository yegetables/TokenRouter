package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/creative"
	"github.com/TokenFlux/TokenRouter/internal/upstream"
)

// OpenAI creative tasks need image bytes for transient delivery even when the
// generation response supplies only a signed URL. The adapter first reads and
// closes the generation response, then prefers a usable base64 result or
// downloads the first nonblank image URL through a separate callback. That
// callback owns download transport and credential isolation.
//
// Generation uses JSON and editing uses multipart references with an optional
// mask, while both request one PNG result. GPT Image bodies omit the legacy
// response_format field and DALL-E names include it. URL downloads have
// bounded retries under the execution context, but exhausted downloads and
// unusable successful response bodies produce nonretryable result errors so
// the worker does not submit another generation for those delivery failures.

// ExecuteOpenAI submits one generation or edit request and returns the
// selected image bytes. Callers provide usable OpenAI callbacks and validate
// creative operation and model parameters before execution.
//
// Generation authentication and provider headers belong to the POST request.
// The response body is read through the shared sixty-four-MiB limiter and
// closed before result URL fetching uses the selected transport pool. HTTP
// failures use status-class retry decisions, while a successful response with
// unusable image delivery returns the nonretryable result classifications
// produced by parseOpenAIImageOutputs.
func (e *Target) ExecuteOpenAI(ctx context.Context, run creative.CreativeRun, payload creative.CreativeRunPayload, upstreamModel string) ([]creative.CreativeOutput, error) {
	if e.OpenAI == nil {
		return nil, errors.New("creative openai gateway is not configured")
	}
	endpoint := upstream.OpenAIImagesGenerationsEndpoint
	if run.Operation != creative.CreativeOperationGenerate {
		endpoint = upstream.OpenAIImagesEditsEndpoint
	}
	body, contentType, err := BuildCreativeOpenAIRequestBody(run, payload, upstreamModel)
	if err != nil {
		return nil, err
	}
	token, err := e.OpenAI.Token(ctx)
	if err != nil {
		return nil, creative.CreativeHTTPStatusError(0, err.Error())
	}
	targetURL, err := e.CreativeOpenAIURL(endpoint)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req = e.OpenAI.Prepare(req)
	authHeaders, err := e.OpenAI.AuthHeaders(ctx, token)
	if err != nil {
		return nil, creative.CreativeHTTPStatusError(0, err.Error())
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	if strings.TrimSpace(contentType) != "" {
		req.Header.Set("Content-Type", contentType)
	}
	// Authentication headers and protocol Content-Type are prepared before the
	// provider override callback. Apply it last so configured provider header
	// values take precedence on the generation request sent by this transport.
	e.OpenAI.ApplyHeaders(req.Header)

	resp, err := e.OpenAI.Do(req)
	if err != nil {
		return nil, creative.CreativeHTTPStatusError(0, err.Error())
	}
	// The shared reader returns a successful prefix when this cap is reached.
	// Image parsing therefore receives bounded JSON and determines whether that
	// prefix still contains a usable result.
	respBody, err := ReadCreativeUpstreamBody(resp.Body, 64<<20)
	// Image download uses a separate callback after this generation read. Close
	// the response first so the provider pool can supply a connection to that
	// download; a close error does not replace the read or HTTP result.
	_ = resp.Body.Close()
	if err != nil {
		return nil, creative.CreativeHTTPStatusError(0, err.Error())
	}
	if resp.StatusCode >= 400 {
		return nil, creative.CreativeHTTPStatusError(resp.StatusCode, upstream.ExtractErrorMessage(respBody))
	}
	return e.parseOpenAIImageOutputs(ctx, respBody)
}

// parseOpenAIImageOutputs selects usable base64 bytes or downloads the first
// nonblank result URL. Invalid JSON and responses with no usable base64 or URL
// return INVALID_IMAGE_RESPONSE. Callers supply nonnil OpenAI options for URL
// handling; a missing FetchImage callback returns IMAGE_DOWNLOAD_FAILED.
//
// A URL receives at most three fetch attempts, separated by one-second waits
// and governed by ctx. A successful fetch still needs nonempty decodable
// bytes. Failure of that first URL ends with IMAGE_DOWNLOAD_FAILED rather than
// trying later URL entries. Both result classifications retain fixed public
// messages and stop worker generation retries, while cancellation stops
// download waiting and further attempts.
func (e *Target) parseOpenAIImageOutputs(ctx context.Context, body []byte) ([]creative.CreativeOutput, error) {
	if !gjson.ValidBytes(body) {
		return nil, creative.CreativeImageResultError("INVALID_IMAGE_RESPONSE", "creative platform returned invalid image response")
	}
	// The shared base64 parser uses retryable errors for malformed image content.
	// Consume only its successful result here, then resolve URL delivery or return
	// a terminal image-result error for this already received generation response.
	if outputs, err := ParseCreativeOpenAIImageOutputs(body); err == nil {
		return outputs, nil
	}
	for _, item := range gjson.GetBytes(body, "data").Array() {
		rawURL := strings.TrimSpace(item.Get("url").String())
		if rawURL == "" {
			continue
		}
		if e.OpenAI.FetchImage != nil {
			// Retry this image fetch under the execution context
			// while retaining its
			// signed URL verbatim. The generation POST has
			// completed and is not part
			// of these attempts, so a transient download error does
			// not generate again.
			for attempt := 0; attempt < 3 && ctx.Err() == nil; attempt++ {
				encoded, err := e.OpenAI.FetchImage(ctx, rawURL)
				if err == nil {
					decoded, decodeErr := DecodeBase64Image(encoded)
					if decodeErr == nil && len(decoded.Bytes) > 0 {
						return []creative.CreativeOutput{{Index: 0, Bytes: decoded.Bytes, Mime: decoded.Mime}}, nil
					}
				}
				if attempt < 2 {
					timer := time.NewTimer(time.Second)
					select {
					case <-ctx.Done():
						timer.Stop()
					case <-timer.C:
					}
				}
			}
		}
		// A download failure can contain the signed address supplied by
		// the response.
		// Return fixed result text so task error metadata can be
		// exposed without that
		// supplied address or its signature.
		return nil, creative.CreativeImageResultError("IMAGE_DOWNLOAD_FAILED", "creative image download failed")
	}
	return nil, creative.CreativeImageResultError("INVALID_IMAGE_RESPONSE", "creative platform returned no decodable image output")
}

// CreativeOpenAIURL delegates generation endpoint resolution to the selected
// OpenAI callback.
func (e *Target) CreativeOpenAIURL(endpoint string) (string, error) { return e.OpenAI.URL(endpoint) }

// BuildCreativeOpenAIRequestBody returns the encoded OpenAI body and its
// content type. Generate uses JSON, while every other operation uses
// multipart, so callers validate operation names and source requirements
// before calling.
//
// Both representations carry the fixed upstream model, prompt, size mapping,
// and one PNG output. Only DALL-E names request the legacy base64
// response_format field. Editing repeats the image form field for sources and
// adds a supplied mask; closing the multipart writer completes its boundary
// before bytes and content type are returned. Optional quality and background
// are forwarded after trimming.
func BuildCreativeOpenAIRequestBody(run creative.CreativeRun, payload creative.CreativeRunPayload, upstreamModel string) ([]byte, string, error) {
	if run.Operation == creative.CreativeOperationGenerate {
		bodyMap := map[string]any{
			"model":  upstreamModel,
			"prompt": payload.Prompt,
			"n":      1,
			"size":   CreativeOpenAIRequestImageSize(upstreamModel, run.ImageSize, run.AspectRatio),
		}
		// output_format is a GPT Image field. A registered third-party endpoint
		// can reject an unknown field, so only native models receive it.
		if CreativeOpenAISendsOutputFormat(upstreamModel) {
			bodyMap["output_format"] = "png"
		}
		// DALL-E selects base64 delivery with its legacy response_format field,
		// and a registered model may need it when its endpoint returns a URL by
		// default. GPT Image keeps the PNG output_format request above instead.
		if CreativeOpenAIUsesResponseFormat(upstreamModel) {
			bodyMap["response_format"] = "b64_json"
		}
		if quality := strings.TrimSpace(payload.Quality); quality != "" {
			bodyMap["quality"] = quality
		}
		if background := strings.TrimSpace(payload.Background); background != "" {
			bodyMap["background"] = background
		}
		body, err := json.Marshal(bodyMap)
		if err != nil {
			return nil, "", err
		}
		return body, "application/json", nil
	}

	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for i, image := range payload.Sources {
		part, err := writer.CreateFormFile("image", fmt.Sprintf("source_%d.%s", i, CreativeFileExtension(image.Mime)))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(image.Bytes); err != nil {
			return nil, "", err
		}
	}
	if payload.Mask != nil {
		part, err := writer.CreateFormFile("mask", "mask."+CreativeFileExtension(payload.Mask.Mime))
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(payload.Mask.Bytes); err != nil {
			return nil, "", err
		}
	}
	if err := writer.WriteField("model", upstreamModel); err != nil {
		return nil, "", err
	}
	if err := writer.WriteField("prompt", payload.Prompt); err != nil {
		return nil, "", err
	}
	// Multipart edits use the same model-family response option as JSON generation.
	// Include the legacy base64 field for DALL-E while GPT Image receives the PNG
	// output_format written below.
	if CreativeOpenAIUsesResponseFormat(upstreamModel) {
		if err := writer.WriteField("response_format", "b64_json"); err != nil {
			return nil, "", err
		}
	}
	if CreativeOpenAISendsOutputFormat(upstreamModel) {
		if err := writer.WriteField("output_format", "png"); err != nil {
			return nil, "", err
		}
	}
	if err := writer.WriteField("size", CreativeOpenAIRequestImageSize(upstreamModel, run.ImageSize, run.AspectRatio)); err != nil {
		return nil, "", err
	}
	if err := writer.WriteField("n", "1"); err != nil {
		return nil, "", err
	}
	if quality := strings.TrimSpace(payload.Quality); quality != "" {
		if err := writer.WriteField("quality", quality); err != nil {
			return nil, "", err
		}
	}
	if background := strings.TrimSpace(payload.Background); background != "" {
		if err := writer.WriteField("background", background); err != nil {
			return nil, "", err
		}
	}
	// Closing writes the final multipart boundary. Return body bytes only after
	// that succeeds so the edit endpoint receives a complete form containing the
	// source images, optional mask, and generation fields.
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), writer.FormDataContentType(), nil
}

// CreativeOpenAIUsesResponseFormat reports whether a trimmed lowercase model
// begins with dall-e. JSON and multipart builders use that check to send the
// legacy base64 response_format field for those model names.
// CreativeOpenAIUsesResponseFormat reports whether the request needs the legacy
// response_format field for base64 delivery. DALL-E always needs it, and a
// registered third-party model needs it when its endpoint returns a URL by
// default.
func CreativeOpenAIUsesResponseFormat(model string) bool {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "dall-e") {
		return true
	}
	profile := creative.CreativeOpenAICompatImageProfileFor(model)
	return profile != nil && profile.RequireBase64Response
}

// CreativeOpenAISendsOutputFormat reports whether the request may carry the GPT
// Image output_format field. A registered third-party endpoint can reject that
// field, so it is left off for those models.
func CreativeOpenAISendsOutputFormat(model string) bool {
	return !creative.IsCreativeOpenAICompatImageModel(model)
}

// CreativeOpenAIRequestImageSize returns the size value for an
// OpenAI-compatible images request. A registered model that encodes size as a
// ratio receives the requested ratio when the catalogue lists it, else the
// first registered ratio; every other model receives a WIDTHxHEIGHT pair.
func CreativeOpenAIRequestImageSize(model, imageSize, aspectRatio string) string {
	profile := creative.CreativeOpenAICompatImageProfileFor(model)
	if profile == nil || !profile.SizeAsRatio {
		return CreativeOpenAIImageSize(imageSize, aspectRatio)
	}
	// The creative capability helper exposes the catalogue's ratio list for this
	// model, so the fallback ratio comes from the same registration.
	ratios := creative.CreativeCapabilitiesForModel(creative.PlatformOpenAI, model).AspectRatios
	ratio := strings.TrimSpace(aspectRatio)
	if slices.Contains(ratios, ratio) {
		return ratio
	}
	if len(ratios) > 0 {
		return ratios[0]
	}
	return "1:1"
}

// ParseCreativeOpenAIImageOutputs returns the first nonempty decodable base64
// image from data. Empty arrays or arrays with no usable item return a
// retryable 502 creative error; OpenAI result handling reclassifies that
// failure before trying a URL.
func ParseCreativeOpenAIImageOutputs(body []byte) ([]creative.CreativeOutput, error) {
	data := gjson.GetBytes(body, "data")
	if !data.IsArray() || len(data.Array()) == 0 {
		return nil, creative.CreativeHTTPStatusError(http.StatusBadGateway, "upstream returned no image output")
	}
	outputs := make([]creative.CreativeOutput, 0, len(data.Array()))
	for _, item := range data.Array() {
		b64 := strings.TrimSpace(item.Get("b64_json").String())
		if b64 == "" {
			continue
		}
		decoded, err := DecodeBase64Image(b64)
		// Skip unusable base64 entries while preserving the first later decodable
		// result. URL delivery is evaluated separately by the OpenAI target when
		// this shared parser finds no usable base64 image.
		if err != nil || len(decoded.Bytes) == 0 {
			continue
		}
		outputs = append(outputs, creative.CreativeOutput{Index: len(outputs), Bytes: decoded.Bytes, Mime: decoded.Mime})
		break
	}
	if len(outputs) == 0 {
		return nil, creative.CreativeHTTPStatusError(http.StatusBadGateway, "upstream returned no decodable image output")
	}
	return outputs, nil
}

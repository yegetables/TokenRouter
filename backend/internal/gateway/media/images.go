package media

import (
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	providercore "github.com/TokenFlux/TokenRouter/internal/provider"
	upstreamcore "github.com/TokenFlux/TokenRouter/internal/upstream"
)

const (
	ImageCapabilityBasic  ImageCapability = "images-basic"
	ImageCapabilityNative ImageCapability = "images-native"
)

type ImageCapability = providercore.OpenAIImagesCapability

type ImageUpload = upstreamcore.ImageUpload

type ImageRequest struct {
	Endpoint           string
	ContentType        string
	Multipart          bool
	Model              string
	ExplicitModel      bool
	Prompt             string
	Stream             bool
	N                  int
	Size               string
	ExplicitSize       bool
	SizeTier           string
	ResponseFormat     string
	Quality            string
	Background         string
	OutputFormat       string
	Moderation         string
	InputFidelity      string
	Style              string
	OutputCompression  *int
	PartialImages      *int
	HasMask            bool
	HasNativeOptions   bool
	RequiredCapability ImageCapability
	InputImageURLs     []string
	MaskImageURL       string
	Uploads            []ImageUpload
	MaskUpload         *ImageUpload
	Body               []byte
	BodyHash           string `json:"-"`
}

// ParseImageRequest 解析图片请求，validateModel 决定是否同时校验模型。
func ParseImageRequest(endpoint, contentType string, body []byte, validateModel bool) (*ImageRequest, error) {
	value, err := upstreamcore.ParseImageRequest(endpoint, contentType, body)
	if err != nil {
		return nil, err
	}
	req := &ImageRequest{}
	ApplyNativeImageRequest(req, value)
	req.SizeTier = NormalizeImageSizeTier(req.Size)
	req.RequiredCapability = ClassifyImageCapability(req)
	if validateModel {
		if err := req.ValidateRoutingModel(req.Model); err != nil {
			return nil, err
		}
	}
	return req, nil
}

// ImageExecutionPath 根据提供商类型选择 API Key 或 OAuth 执行方式。
func ImageExecutionPath(providerType string) (bool, error) {
	switch providerType {
	case "apikey":
		return false, nil
	case "oauth", "setup-token":
		return true, nil
	default:
		return false, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

func (r *ImageRequest) ModerationBody() []byte {
	return NativeImageRequest(r).ModerationBody()
}
func (r *ImageRequest) IsEdits() bool { return NativeImageRequest(r).IsEdits() }
func (r *ImageRequest) StickySessionSeed() string {
	return NativeImageRequest(r).StickySessionSeed()
}

// ValidateRoutingModel 使用分组映射后的模型 C 校验 Images 端点，并同步提供商选择所需的图片能力。
func (r *ImageRequest) ValidateRoutingModel(routingModel string) error {
	if err := ValidateImageModel(routingModel); err != nil {
		return err
	}
	if r == nil {
		return nil
	}
	routed := *r
	routed.Model = strings.TrimSpace(routingModel)
	r.RequiredCapability = ClassifyImageCapability(&routed)
	return nil
}

// IsImageGenerationModel 判断模型是否为可承接图片入口的生图模型：原生生图族、已登记
// 的第三方兼容模型或名称含 image。账号选路、模型目录投影与图片计费共用此判定，
// 名单由 internal/upstream 唯一持有。
func IsImageGenerationModel(model string) bool {
	return upstreamcore.IsImageGenerationModel(model)
}

// IsImageBillingModelAlias 保留旧名，口径与 IsImageGenerationModel 一致。
func IsImageBillingModelAlias(model string) bool {
	return IsImageGenerationModel(model)
}

func IsGPTImageGenerationModel(model string) bool {
	return upstreamcore.IsGPTImageGenerationModel(model)
}

func IsGrokImageGenerationModel(model string) bool {
	return upstreamcore.IsGrokImageGenerationModel(model)
}

func ValidateImageModel(model string) error {
	model = strings.TrimSpace(model)
	// 生图模型判定已统一到 IsImageGenerationModel：原生生图族或名称含 image 的
	// OpenAI 兼容第三方模型都放行，普通文本模型仍被拒。
	if IsImageBillingModelAlias(model) {
		return nil
	}
	if model == "" {
		return fmt.Errorf("images endpoint requires an image model")
	}
	return fmt.Errorf("images endpoint requires an image model, got %q", model)
}

func ClassifyImageCapability(req *ImageRequest) ImageCapability {
	if req == nil {
		return ImageCapabilityNative
	}
	if req.ExplicitModel || req.ExplicitSize {
		return ImageCapabilityNative
	}
	model := strings.ToLower(strings.TrimSpace(req.Model))
	if !strings.HasPrefix(model, "gpt-image-") {
		return ImageCapabilityNative
	}
	if req.Stream || req.N != 1 || req.HasMask || req.HasNativeOptions {
		return ImageCapabilityNative
	}
	if req.IsEdits() && !req.Multipart {
		return ImageCapabilityNative
	}
	if req.ResponseFormat != "" && req.ResponseFormat != "b64_json" {
		return ImageCapabilityNative
	}
	return ImageCapabilityBasic
}

func NormalizeImageSizeTier(size string) string {
	return pricing.NormalizeImageBillingTierOrDefault(size)
}

func NativeImageRequest(value *ImageRequest) *upstreamcore.ImageRequest {
	if value == nil {
		return nil
	}
	return &upstreamcore.ImageRequest{
		Endpoint:          value.Endpoint,
		ContentType:       value.ContentType,
		Multipart:         value.Multipart,
		Model:             value.Model,
		ExplicitModel:     value.ExplicitModel,
		Prompt:            value.Prompt,
		Stream:            value.Stream,
		N:                 value.N,
		Size:              value.Size,
		ExplicitSize:      value.ExplicitSize,
		SizeTier:          value.SizeTier,
		ResponseFormat:    value.ResponseFormat,
		Quality:           value.Quality,
		Background:        value.Background,
		OutputFormat:      value.OutputFormat,
		Moderation:        value.Moderation,
		InputFidelity:     value.InputFidelity,
		Style:             value.Style,
		OutputCompression: value.OutputCompression,
		PartialImages:     value.PartialImages,
		HasMask:           value.HasMask,
		HasNativeOptions:  value.HasNativeOptions,
		InputImageURLs:    value.InputImageURLs,
		MaskImageURL:      value.MaskImageURL,
		Uploads:           value.Uploads,
		MaskUpload:        value.MaskUpload,
		Body:              value.Body,
		BodyHash:          value.BodyHash,
	}
}

func ApplyNativeImageRequest(target *ImageRequest, value *upstreamcore.ImageRequest) {
	if target == nil || value == nil {
		return
	}
	target.Endpoint = value.Endpoint
	target.ContentType = value.ContentType
	target.Multipart = value.Multipart
	target.Model = value.Model
	target.ExplicitModel = value.ExplicitModel
	target.Prompt = value.Prompt
	target.Stream = value.Stream
	target.N = value.N
	target.Size = value.Size
	target.ExplicitSize = value.ExplicitSize
	target.SizeTier = value.SizeTier
	target.ResponseFormat = value.ResponseFormat
	target.Quality = value.Quality
	target.Background = value.Background
	target.OutputFormat = value.OutputFormat
	target.Moderation = value.Moderation
	target.InputFidelity = value.InputFidelity
	target.Style = value.Style
	target.OutputCompression = value.OutputCompression
	target.PartialImages = value.PartialImages
	target.HasMask = value.HasMask
	target.HasNativeOptions = value.HasNativeOptions
	target.InputImageURLs = value.InputImageURLs
	target.MaskImageURL = value.MaskImageURL
	target.Uploads = value.Uploads
	target.MaskUpload = value.MaskUpload
	target.Body = value.Body
	target.BodyHash = value.BodyHash
}

// ResolveImageModels 在已选提供商上按原顺序校验分组映射模型、提供商映射及上游模型。
func ResolveImageModels(requested, groupMapped, fallback string, resolve func(string) string) (string, string, error) {
	model := strings.TrimSpace(requested)
	if mapped := strings.TrimSpace(groupMapped); mapped != "" {
		model = mapped
	}
	if model == "" {
		model = fallback
	}
	if err := ValidateImageModel(model); err != nil {
		return "", "", err
	}
	upstream := resolve(model)
	if err := ValidateImageModel(upstream); err != nil {
		return "", "", err
	}
	return model, upstream, nil
}

// ImageOutcome 决定已观测图片是否足以保留失败结果，并维持不同传输的计数回退。
// OAuth 已完成的正常响应允许使用请求张数；API Key 的 SSE 则必须有实际产出。
func ImageOutcome(stream, oauth, eventStream bool, requested, observed int, failure error) (int, bool) {
	if failure != nil && (!stream || observed <= 0) {
		return 0, false
	}
	count := observed
	if failure == nil && count <= 0 && (oauth || !stream || !eventStream) {
		count = requested
	}
	return count, true
}

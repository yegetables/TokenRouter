import { apiClient } from './client'

// fork 专属：请求载荷快照（self_request_payloads 表）。
export interface SelfRequestPayload {
  client_request_id: string
  request_headers?: string | null
  response_headers?: string | null
  request_body?: string | null
  response_body?: string | null
  request_body_truncated: boolean
  response_body_truncated: boolean
  status_code: number
  request_path?: string | null
}

export const selfAPI = {
  async getRequestPayload(clientRequestId: string): Promise<SelfRequestPayload> {
    // apiClient 拦截器已解包 { code, message, data } 信封。
    const { data } = await apiClient.get<SelfRequestPayload>(`/admin/self/request-payloads/${encodeURIComponent(clientRequestId)}`)
    return data
  },
}

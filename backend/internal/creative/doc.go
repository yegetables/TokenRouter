// Package creative 提供创作台图片任务的模型目录、提交、执行和结果交付。
//
// 阅读入口：
//   - public.go：列出可用模型，校验请求并创建任务。
//   - worker.go：消费队列，调用提供商并推进任务状态。
//   - results.go：保存输出、结算费用和处理失败任务。
//
// 文件分组：
//   - public.go、queries.go、run.go：公开操作、任务查询和任务数据结构。
//   - model_*.go、operation_protocol.go：模型设置、分组策略和图片操作协议。
//   - openai_compat_image.go：登记第三方 OpenAI 兼容生图模型的契约。
//   - executor.go、execution_*.go：准备上游执行、归一化输出和判断重试错误。
//   - worker.go、runtime.go、queue.go：任务消费、工作池和队列接口。
//   - billing.go、results.go、result_delivery.go：资金预占、结算和临时输出交付。
//   - runtime_settings.go、settings_participant.go、admin_settings_read.go：读取和校验创作台配置。
package creative

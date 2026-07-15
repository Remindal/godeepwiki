// Package embed 向量模型 Provider 抽象与实现。
package embed

import "context"

// Embedder 向量模型抽象（基线 §7，冻结签名）。
// 任何官方 SDK 类型禁止出现在本签名与返回值中（硬约束 #17）。
type Embedder interface {
	// Embed 批量向量化；实现方内部按配置 batch_size 分批、带超时与 SDK 内置/外包指数退避重试（硬约束 #7）。
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimensions() int      // 向量维度（配置探测/构造时确定，运行期不可变）
	ProviderName() string // openai|dashscope|siliconflow|ollama|voyage
	ModelName() string
}

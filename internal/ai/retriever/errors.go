package retriever

import "errors"

// ES 连接与检索相关的可识别错误。错误信息只包含依赖名和净化后的描述，
// 不包含地址、证书或凭证。
var (
	errNoESAddress      = errors.New("elasticsearch address is required")
	errNoESIndex        = errors.New("elasticsearch index is required")
	errInvalidDim       = errors.New("embedding dimension must be positive")
	errNoESClient       = errors.New("elasticsearch client is not initialized")
	errNoChunkID        = errors.New("chunk id is required")
	errInvalidVectorDim = errors.New("embedding vector dimension does not match index")
	errEmptyQuery       = errors.New("query must not be empty")
	errNoEmbedder       = errors.New("embedder is required")
	errInvalidTopK      = errors.New("top k must be positive")
)

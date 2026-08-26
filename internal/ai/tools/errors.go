package tools

import "errors"

// 工具层可识别错误。错误信息只包含操作名和净化后的描述，
// 不暴露底层 ES/model 的具体错误。
var (
	errRetrieverUnavailable = errors.New("knowledge search retriever is unavailable")
	errInvalidInput         = errors.New("knowledge search input is invalid")
	errEmptyQuery           = errors.New("query must not be empty")
	errRetrieve             = errors.New("knowledge search failed")
)

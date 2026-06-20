package errors

import (
	stderrors "errors"
	"fmt"
	"sort"
	"strings"
)

// Surface returns the user/LLM-facing text for an error: the first *Error in the chain's clean
// Message plus its Details — NOT err.Error()'s wrapped chain, which leaks the internal Go
// package/method breadcrumbs app layers add via fmt.Errorf("pkg.Method: %w", …) (e.g.
// "functionapp.RunFunction:"). The actionable part for self-correction is Message + Details, never
// the call path (S20). A non-structured error (raw stdlib) has only its text, returned as-is.
//
// This is the single source for every error surface the LLM / a flowrun node / an agent execution
// reads — it was independently copied as the loop's llmErrText (F89) and the scheduler's nodeErrText
// (F104) before a third call site (agent invoke) made the duplication a foundation gap (principle #8).
//
// Surface 返回错误的「用户/LLM 可见」文本：链中第一个 *Error 的干净 Message + Details——**非**
// err.Error() 的包裹链（会泄露 app 层经 fmt.Errorf("pkg.Method: %w", …) 加的内部 Go 包/方法面包屑）。
// 自纠所需是 Message + Details、绝非调用路径（S20）。非结构化错误（裸 stdlib）只有其文本、原样返回。
// 这是 LLM / flowrun 节点 / agent 执行读取的每个错误面的单一来源——曾被独立抄成 loop 的 llmErrText
// （F89）与 scheduler 的 nodeErrText（F104），第三个调用点（agent invoke）使这份重复成了地基缺口（原则 #8）。
func Surface(err error) string {
	if err == nil {
		return ""
	}
	var de *Error
	if !stderrors.As(err, &de) {
		return err.Error()
	}
	msg := de.Message
	if len(de.Details) > 0 {
		parts := make([]string, 0, len(de.Details))
		for k, v := range de.Details {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
		sort.Strings(parts)
		msg += " (" + strings.Join(parts, "; ") + ")"
	}
	return msg
}

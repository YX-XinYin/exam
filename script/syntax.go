package main

import "strings"

// Unconvertible 是题目要求的固定答案：需求不能变成一条可执行的 FOFA 语句。
const Unconvertible = "该需求不能直接转换为FOFA搜索语句"

func escape(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	return v
}

func has(field, value string) string {
	return field + `="` + escape(value) + `"`
}

func exact(field, value string) string {
	return field + `=="` + escape(value) + `"`
}

func ne(field, value string) string {
	return field + `!="` + escape(value) + `"`
}

func wild(field, pattern string) string {
	return field + `*="` + escape(pattern) + `"`
}

func boolean(field string, v bool) string {
	if v {
		return field + "=true"
	}
	return field + "=false"
}

func cmp(field, op, value string) string {
	return field + op + `"` + escape(value) + `"`
}

func and(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " && ")
}

func or(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		// && 包进括号，避免和 || 连写时把两侧条件绑错。
		if strings.Contains(p, "&&") && !(strings.HasPrefix(p, "(") && strings.HasSuffix(p, ")")) {
			p = "(" + p + ")"
		}
		out = append(out, p)
	}
	if len(out) == 1 {
		return out[0]
	}
	return "(" + strings.Join(out, " || ") + ")"
}

func anyHas(field string, values ...string) string {
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, has(field, v))
	}
	return or(parts...)
}

func present(field string) string { return ne(field, "") }

func absent(field string) string { return has(field, "") }

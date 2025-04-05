package utils

// FilterUserDefinedCalls 过滤掉白名单中定义的系统内置函数，
// 只保留那些不在白名单中的函数调用。
// 参数 calls 是提取到的函数调用名称列表，
// 参数 whitelist 是一个字典，key 为白名单中的函数名称，值为 true 表示该函数属于内置函数。
func FilterUserDefinedCalls(calls []string, whitelist map[string]bool) []string {
	var filtered []string
	for _, call := range calls {
		// 如果调用名称不在白名单中，则认为是用户自定义函数（或非内置函数），保留它
		if !whitelist[call] {
			filtered = append(filtered, call)
		}
	}
	return filtered
}

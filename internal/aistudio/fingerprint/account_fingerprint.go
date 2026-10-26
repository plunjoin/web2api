package fingerprint

// AccountFingerprint 返回账户固定指纹配置，不存在时按当前版本生成并保存
func AccountFingerprint(storageStatePath, locale, timezone string) (map[string]any, error) {
	return loadAccountFingerprint(storageStatePath, firefoxMajor, locale, timezone)
}

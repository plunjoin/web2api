//go:build web2api_unit

package store

import "testing"

func TestModelEnabledDefaultAndDisable(t *testing.T) {
	s := newTestStore(t)
	enabled, err := s.ModelEnabled("gemini-flash")
	if err != nil || !enabled {
		t.Fatalf("默认应启用: enabled=%v err=%v", enabled, err)
	}
	setting, err := s.SetModelEnabled("gemini-flash", false)
	if err != nil || setting.Enabled {
		t.Fatalf("设为不可用失败: %+v %v", setting, err)
	}
	enabled, err = s.ModelEnabled("gemini-flash")
	if err != nil || enabled {
		t.Fatalf("应不可用: enabled=%v err=%v", enabled, err)
	}
	disabled, err := s.DisabledModels()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := disabled["gemini-flash"]; !ok {
		t.Fatalf("DisabledModels 应包含 gemini-flash: %+v", disabled)
	}
	setting, err = s.SetModelEnabled("gemini-flash", true)
	if err != nil || !setting.Enabled {
		t.Fatalf("恢复可用失败: %+v %v", setting, err)
	}
}

func TestRecordModelCheckPreservesEnabled(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.SetModelEnabled("m1", false); err != nil {
		t.Fatal(err)
	}
	setting, err := s.RecordModelCheck("m1", false, "upstream 503")
	if err != nil {
		t.Fatal(err)
	}
	if setting.Enabled {
		t.Fatal("检查失败不应改写上下架状态")
	}
	if setting.LastCheckOK || setting.LastCheckDetail != "upstream 503" || setting.LastCheckAt == 0 {
		t.Fatalf("检查结果未写入: %+v", setting)
	}
	setting, err = s.RecordModelCheck("m1", true, "pong")
	if err != nil || !setting.LastCheckOK || setting.Enabled {
		t.Fatalf("检查通过应保留不可用: %+v %v", setting, err)
	}
}

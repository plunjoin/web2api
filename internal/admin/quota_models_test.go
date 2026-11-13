package admin

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"web2api/internal/store"
)

// 「未归类」死记录不再展示：model_settings 里历史上检查过的模型（上游目录早已
// 摘掉、所有引擎的实时目录也不再声明）不能变成一个 40 多个模型的假供应商组。
// 快照里的模型照常展示，孤儿记录保留在库里（模型回归时开关/检查历史仍生效）。
func TestHandleModelsHidesDeadOrphanSettings(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "models.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.ReplaceEngineCatalog("workbuddy", []store.CatalogModel{
		{Engine: "workbuddy", Model: "deepseek-v4.1-flash", DisplayName: "DeepSeek-V4.1-Flash"},
	}); err != nil {
		t.Fatal(err)
	}
	// 上游已下线的模型：只剩一条检查记录，不在快照、也不在任何引擎目录里。
	if _, err := st.RecordModelCheck("deepseek-v4.1-flash-sg", false, "没有符合条件的 AI Studio 账户"); err != nil {
		t.Fatal(err)
	}

	a := &API{st: st, logger: log.New(io.Discard, "", 0)}
	r := httptest.NewRequest(http.MethodGet, "/admin/api/models", nil)
	w := httptest.NewRecorder()
	a.handleModels(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Models []struct {
			ID     string `json:"id"`
			Engine string `json:"engine"`
		} `json:"models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, m := range body.Models {
		seen[m.ID] = m.Engine
	}
	if eng, ok := seen["deepseek-v4.1-flash"]; !ok || eng != "workbuddy" {
		t.Fatalf("快照模型应照常展示: %+v", body.Models)
	}
	if _, ok := seen["deepseek-v4.1-flash-sg"]; ok {
		t.Fatalf("死记录不应展示为未归类模型: %+v", body.Models)
	}
}

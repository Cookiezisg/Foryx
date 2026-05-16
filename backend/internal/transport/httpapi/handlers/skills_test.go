package handlers

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"

	skillapp "github.com/sunweilin/forgify/backend/internal/app/skill"
	skilldomain "github.com/sunweilin/forgify/backend/internal/domain/skill"
	middlewarehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/middleware"
)

type skillHandlerHarness struct {
	srv *httptest.Server
	svc *skillapp.Service
	dir string
}

func newSkillsTestServer(t *testing.T) *skillHandlerHarness {
	t.Helper()
	log := zaptest.NewLogger(t)
	dir := t.TempDir()
	svc := skillapp.New(dir, nil, nil, nil, nil, nil, log)
	hd := NewSkillsHandler(svc, log)
	mux := http.NewServeMux()
	hd.Register(mux)
	srv := httptest.NewServer(middlewarehttpapi.InjectUserID(mux))
	t.Cleanup(srv.Close)
	return &skillHandlerHarness{srv: srv, svc: svc, dir: dir}
}

func (h *skillHandlerHarness) seedSkillOnDisk(t *testing.T, name, desc string, fork bool) {
	t.Helper()
	skDir := filepath.Join(h.dir, name)
	if err := os.MkdirAll(skDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	fm := "name: " + name + "\ndescription: " + desc
	if fork {
		fm += "\ncontext: fork\nagent: Explore"
	}
	if err := os.WriteFile(
		filepath.Join(skDir, "SKILL.md"),
		[]byte("---\n"+fm+"\n---\n# Body for "+name+"\nrun the steps."),
		0o644,
	); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
}


func TestSkills_List_Empty(t *testing.T) {
	h := newSkillsTestServer(t)
	resp, err := http.Get(h.srv.URL + "/api/v1/skills")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := envOf[[]*skilldomain.Skill](t, resp.Body)
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestSkills_List_AfterSeed(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "alpha", "alpha desc", false)
	h.seedSkillOnDisk(t, "beta", "beta desc", true)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}

	resp, _ := http.Get(h.srv.URL + "/api/v1/skills")
	got := envOf[[]*skilldomain.Skill](t, resp.Body)
	if len(got) != 2 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Errorf("order: %s/%s", got[0].Name, got[1].Name)
	}
	if got[1].Frontmatter.Context != "fork" {
		t.Errorf("beta frontmatter.context lost: %q", got[1].Frontmatter.Context)
	}
}

func TestSkills_Get_NotFound(t *testing.T) {
	h := newSkillsTestServer(t)
	resp, _ := http.Get(h.srv.URL + "/api/v1/skills/ghost")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSkills_GetBody_ReturnsRawSKILLMD(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "deploy", "deploy step", false)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	resp, _ := http.Get(h.srv.URL + "/api/v1/skills/deploy/body")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := envOf[map[string]any](t, resp.Body)
	body, _ := got["body"].(string)
	if !strings.Contains(body, "# Body for deploy") {
		t.Errorf("body = %q", body)
	}
}


func TestSkills_Create_201(t *testing.T) {
	h := newSkillsTestServer(t)
	body := strings.NewReader(`{
		"name": "new-skill",
		"frontmatter": {"name":"new-skill","description":"created via API"},
		"body": "# Created"
	}`)
	resp, err := http.Post(h.srv.URL+"/api/v1/skills", "application/json", body)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, raw)
	}
	got := envOf[*skilldomain.Skill](t, resp.Body)
	if got.Name != "new-skill" {
		t.Errorf("name = %q", got.Name)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "new-skill", "SKILL.md")); err != nil {
		t.Errorf("SKILL.md not on disk: %v", err)
	}
}

func TestSkills_Create_Conflict_409(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "existing", "existing desc", false)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	body := strings.NewReader(`{"name":"existing","frontmatter":{"description":"x"},"body":"y"}`)
	resp, _ := http.Post(h.srv.URL+"/api/v1/skills", "application/json", body)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("status = %d, want 409", resp.StatusCode)
	}
}

func TestSkills_Create_InvalidName_422(t *testing.T) {
	h := newSkillsTestServer(t)
	body := strings.NewReader(`{"name":"BAD CASE WITH SPACES","frontmatter":{"description":"x"},"body":"y"}`)
	resp, _ := http.Post(h.srv.URL+"/api/v1/skills", "application/json", body)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", resp.StatusCode)
	}
}

func TestSkills_Replace_200(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "deploy", "v1", false)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	body := strings.NewReader(`{
		"frontmatter":{"name":"deploy","description":"v2 description"},
		"body":"# Updated"
	}`)
	req, _ := http.NewRequest(http.MethodPut, h.srv.URL+"/api/v1/skills/deploy", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, raw)
	}
	got := envOf[*skilldomain.Skill](t, resp.Body)
	if got.Description != "v2 description" {
		t.Errorf("Description = %q", got.Description)
	}
}

func TestSkills_Replace_NotFound_404(t *testing.T) {
	h := newSkillsTestServer(t)
	body := strings.NewReader(`{"frontmatter":{"description":"x"},"body":"y"}`)
	req, _ := http.NewRequest(http.MethodPut, h.srv.URL+"/api/v1/skills/ghost", body)
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSkills_Delete_204(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "doomed", "del me", false)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	req, _ := http.NewRequest(http.MethodDelete, h.srv.URL+"/api/v1/skills/doomed", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(h.dir, "doomed")); !os.IsNotExist(err) {
		t.Errorf("doomed dir still exists; stat err = %v", err)
	}
}

func TestSkills_Delete_NotFound_404(t *testing.T) {
	h := newSkillsTestServer(t)
	req, _ := http.NewRequest(http.MethodDelete, h.srv.URL+"/api/v1/skills/ghost", nil)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSkills_Refresh_PicksUpDiskWrite(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "fresh", "fresh desc", false)
	resp, _ := http.Post(h.srv.URL+"/api/v1/skills:refresh", "application/json", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	got := envOf[[]*skilldomain.Skill](t, resp.Body)
	if len(got) != 1 || got[0].Name != "fresh" {
		t.Errorf("got %+v", got)
	}
}

func TestSkills_Import_JSON(t *testing.T) {
	h := newSkillsTestServer(t)
	body := strings.NewReader(`{
		"files": [
			{"name":"imported-a","body":"---\nname: imported-a\ndescription: from-import\n---\nbody-a"},
			{"name":"imported-b","body":"---\nname: imported-b\ndescription: also\n---\nbody-b"}
		]
	}`)
	resp, _ := http.Post(h.srv.URL+"/api/v1/skills:import", "application/json", body)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, raw)
	}
	res := envOf[skillapp.ImportResult](t, resp.Body)
	if len(res.Imported) != 2 {
		t.Errorf("Imported = %v, want 2", res.Imported)
	}
	if len(res.Conflicts) != 0 || len(res.Errors) != 0 {
		t.Errorf("unexpected conflicts/errors: %+v", res)
	}
}

func TestSkills_Import_Conflict_NoOverwrite(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "exists", "existing", false)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	body := strings.NewReader(`{
		"files": [{"name":"exists","body":"---\nname: exists\ndescription: replacement\n---\nnew-body"}]
	}`)
	resp, _ := http.Post(h.srv.URL+"/api/v1/skills:import", "application/json", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	res := envOf[skillapp.ImportResult](t, resp.Body)
	if len(res.Conflicts) != 1 || res.Conflicts[0] != "exists" {
		t.Errorf("Conflicts = %v", res.Conflicts)
	}
	if len(res.Imported) != 0 {
		t.Errorf("Imported should be empty without overwrite; got %v", res.Imported)
	}
}

func TestSkills_Import_OverwriteForce(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "exists", "v1", false)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	body := strings.NewReader(`{
		"files": [{"name":"exists","body":"---\nname: exists\ndescription: v2\n---\nnew-body"}]
	}`)
	resp, _ := http.Post(h.srv.URL+"/api/v1/skills:import?overwrite=true", "application/json", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	res := envOf[skillapp.ImportResult](t, resp.Body)
	if len(res.Imported) != 1 {
		t.Errorf("Imported = %v, want [exists]", res.Imported)
	}
	got, _ := h.svc.Get(context.Background(), "exists")
	if got.Description != "v2" {
		t.Errorf("Description = %q after overwrite, want v2", got.Description)
	}
}

func TestSkills_Import_Multipart(t *testing.T) {
	h := newSkillsTestServer(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	w, _ := mw.CreateFormFile("file", "from-multipart.md")
	_, _ = w.Write([]byte("---\nname: from-multipart\ndescription: via multipart\n---\nbody"))
	_ = mw.Close()

	resp, err := http.Post(h.srv.URL+"/api/v1/skills:import", mw.FormDataContentType(), &buf)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, raw)
	}
	res := envOf[skillapp.ImportResult](t, resp.Body)
	if len(res.Imported) != 1 || res.Imported[0] != "from-multipart" {
		t.Errorf("Imported = %v", res.Imported)
	}
}

func TestSkills_Import_EmptyRejected(t *testing.T) {
	h := newSkillsTestServer(t)
	resp, _ := http.Post(h.srv.URL+"/api/v1/skills:import", "application/json", strings.NewReader(`{"files":[]}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSkills_Invoke_NonFork_ReturnsBody(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "deploy", "deploy steps", false)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	body := strings.NewReader(`{"arguments":[]}`)
	u := h.srv.URL + "/api/v1/skills/" + url.PathEscape("deploy:invoke")
	resp, _ := http.Post(u, "application/json", body)
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d; body=%s", resp.StatusCode, raw)
	}
	got := envOf[map[string]any](t, resp.Body)
	if !strings.Contains(got["result"].(string), "# Body for deploy") {
		t.Errorf("result missing body content: %v", got)
	}
}

func TestSkills_Invoke_NotFound_404(t *testing.T) {
	h := newSkillsTestServer(t)
	u := h.srv.URL + "/api/v1/skills/" + url.PathEscape("ghost:invoke")
	resp, _ := http.Post(u, "application/json", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestSkills_NameAction_UnknownAction_400(t *testing.T) {
	h := newSkillsTestServer(t)
	h.seedSkillOnDisk(t, "deploy", "x", false)
	if err := h.svc.Scan(context.Background()); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	u := h.srv.URL + "/api/v1/skills/" + url.PathEscape("deploy:bogus")
	resp, _ := http.Post(u, "application/json", nil)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

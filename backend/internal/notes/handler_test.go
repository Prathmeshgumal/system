package notes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeService lets handler tests choose what the service returns.
type fakeService struct {
	createErr error
	list      []Note
	listErr   error
}

func (f *fakeService) Create(ctx context.Context, body string) (Note, error) {
	if f.createErr != nil {
		return Note{}, f.createErr
	}
	return Note{ID: 7, Body: body, CreatedAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}, nil
}

func (f *fakeService) ListLatest(ctx context.Context) ([]Note, error) { return f.list, f.listErr }

func do(svc service, method, path, body string) *httptest.ResponseRecorder {
	mux := http.NewServeMux()
	NewHandler(svc).Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	return rec
}

func errorCode(t *testing.T, rec *httptest.ResponseRecorder) (string, map[string]string) {
	t.Helper()
	var env struct {
		Error struct {
			Code   string            `json:"code"`
			Fields map[string]string `json:"fields"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("not our error shape: %v", err)
	}
	return env.Error.Code, env.Error.Fields
}

func TestCreate_Success_Returns201WithTheNote(t *testing.T) {
	rec := do(&fakeService{}, http.MethodPost, "/api/notes", `{"body":"hello"}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("code = %d, want 201; body %s", rec.Code, rec.Body)
	}
	var n Note
	if err := json.NewDecoder(rec.Body).Decode(&n); err != nil {
		t.Fatal(err)
	}
	if n.ID != 7 || n.Body != "hello" || n.CreatedAt.IsZero() {
		t.Fatalf("unexpected note: %+v", n)
	}
}

func TestCreate_BadJSON_Returns400InvalidJSON(t *testing.T) {
	rec := do(&fakeService{}, http.MethodPost, "/api/notes", `{"body":`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	if code, _ := errorCode(t, rec); code != "invalid_json" {
		t.Fatalf("error code = %q, want invalid_json", code)
	}
}

func TestCreate_InvalidBody_Returns400WithFieldError(t *testing.T) {
	svc := &fakeService{createErr: &ValidationError{Fields: map[string]string{"body": "must not be empty"}}}
	rec := do(svc, http.MethodPost, "/api/notes", `{"body":""}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code = %d, want 400", rec.Code)
	}
	code, fields := errorCode(t, rec)
	if code != "validation_failed" || fields["body"] != "must not be empty" {
		t.Fatalf("got code %q fields %v", code, fields)
	}
}

func TestCreate_DatabaseUnavailable_Returns503(t *testing.T) {
	rec := do(&fakeService{createErr: ErrUnavailable}, http.MethodPost, "/api/notes", `{"body":"hi"}`)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code = %d, want 503", rec.Code)
	}
	if code, _ := errorCode(t, rec); code != "unavailable" {
		t.Fatalf("error code = %q, want unavailable", code)
	}
}

func TestCreate_UnexpectedError_Returns500WithoutLeakingIt(t *testing.T) {
	rec := do(&fakeService{createErr: errors.New("pq: secret internal detail")}, http.MethodPost, "/api/notes", `{"body":"hi"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret internal detail") {
		t.Fatalf("internal error text leaked to the client: %s", rec.Body)
	}
}

func TestList_ReturnsNotesWrapper(t *testing.T) {
	svc := &fakeService{list: []Note{{ID: 2, Body: "b"}, {ID: 1, Body: "a"}}}
	rec := do(svc, http.MethodGet, "/api/notes", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200", rec.Code)
	}
	var body struct {
		Notes []Note `json:"notes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Notes) != 2 || body.Notes[0].ID != 2 {
		t.Fatalf("unexpected notes: %+v", body.Notes)
	}
}

func TestList_Empty_ReturnsEmptyArrayNotNull(t *testing.T) {
	rec := do(&fakeService{list: nil}, http.MethodGet, "/api/notes", "")

	if !strings.Contains(rec.Body.String(), `"notes":[]`) {
		t.Fatalf("want \"notes\":[] so the frontend can always loop over it, got %s", rec.Body)
	}
}

package www

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChineseResourcesAndLegacyURLs(t *testing.T) {
	for old, translated := range aliases {
		t.Run(old, func(t *testing.T) {
			want, err := Static.ReadFile(translated)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Static.ReadFile(old)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatal("旧地址与中文资源内容不一致")
			}
			for _, name := range []string{old, translated} {
				response := httptest.NewRecorder()
				http.FileServer(http.FS(Static)).ServeHTTP(response, httptest.NewRequest("GET", "/"+name, nil))
				if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), want) {
					t.Fatalf("资源 %s 未正确提供：HTTP %d", name, response.Code)
				}
			}
		})
	}
}

package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"gemrelay/internal/gateway"
)

// handleAdminQuota 查询网页端账号额度，失败通过 ok:false 表达，不影响生成接口。
func handleAdminQuota(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeJSON(w, 405, map[string]string{"error": "method not allowed"})
		return
	}
	q := r.URL.Query()
	var accountID int64
	if raw := strings.TrimSpace(q.Get("account_id")); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, 400, map[string]string{"error": "bad account_id"})
			return
		}
		accountID = id
	}
	writeJSON(w, 200, gateway.GetQuota(q.Get("refresh") == "1", accountID))
}

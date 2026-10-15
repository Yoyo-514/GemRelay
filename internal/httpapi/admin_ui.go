package httpapi

import (
	"io/fs"
	"net/http"

	"gemini-web2api/web/admin"
)

func handleAdminUI(w http.ResponseWriter, r *http.Request) {
	// 面板里的地址全是相对的，好让它在反代的子路径下（example.com/gemini/admin）
	// 也能用。相对地址是按**文档 URL 的目录**解析的，所以 /admin 和 /admin/ 会解析
	// 出不同结果 —— 前者的目录是上一层。统一重定向到带斜杠那个形式。
	//
	// Location 必须是相对值：写成 "/admin/" 的话，反代下浏览器会跳到站点根的
	// /admin/，前缀 /gemini 就丢了。这里不用 http.Redirect —— 它会把相对目标
	// 按请求路径展开成绝对路径，正好毁掉这一点。
	path := r.URL.Path
	if path == "/admin" {
		w.Header().Set("Location", "admin/")
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}
	if path == "/admin/" {
		path = "index.html"
	} else {
		path = path[len("/admin/"):]
	}
	data, err := fs.ReadFile(admin.Files, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// 文件名不带内容指纹，部署新二进制后应重新验证，避免 HTML 与脚本版本错配。
	w.Header().Set("Cache-Control", "no-cache")
	switch {
	case len(path) > 5 && path[len(path)-5:] == ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case len(path) > 3 && path[len(path)-3:] == ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case len(path) > 4 && path[len(path)-4:] == ".css":
		w.Header().Set("Content-Type", "text/css")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	w.Write(data)
}

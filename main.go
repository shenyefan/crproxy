package main

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

var client = &http.Client{
	Transport: &http.Transport{
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	},
}

const authURL = "https://auth.docker.io"

// routeByHosts 根据主机名选择对应的上游地址，返回上游地址
func routeByHosts(host string) (string, bool) {
	routes := map[string]string{
		"gcr":  "gcr.io",
		"k8s":  "registry.k8s.io",
		"ghcr": "ghcr.io",
	}
	if r, ok := routes[host]; ok {
		return r, false
	}
	return "registry-1.docker.io", true
}

// searchInterface 返回 Docker Hub 镜像搜索页面 HTML
func searchInterface() string {
	return `<!DOCTYPE html>
	<html>
	<head>
		<title>Docker Hub 镜像</title>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<style>
		* {
			box-sizing: border-box;
			margin: 0;
			padding: 0;
		}

		body {
			display: flex;
			flex-direction: column;
			justify-content: center;
			align-items: center;
			min-height: 100vh;
			margin: 0;
			background: linear-gradient(120deg, #1a90ff 0%, #003eb3 100%);
			padding: 20px;
		}

		.container {
			text-align: center;
			width: 100%;
			max-width: 800px;
			padding: 0 20px;
			margin: 0 auto;
			display: flex;
			flex-direction: column;
			justify-content: center;
			min-height: 70vh;
		}
			
		@keyframes octocat-wave {
			0%, 100% {
				transform: rotate(0);
			}
			20%, 60% {
				transform: rotate(-25deg);
			}
			40%, 80% {
				transform: rotate(10deg);
			}
		}

		.logo {
			margin-bottom: 30px;
			transition: transform 0.3s ease;
		}

		.logo:hover {
			transform: scale(1.05);
		}

		.search-container {
			display: flex;
			align-items: stretch;
			width: 100%;
			max-width: 600px;
			margin: 0 auto;
			height: 50px;
		}

		#search-input {
			flex: 1;
			padding: 15px 20px;
			font-size: 16px;
			border: none;
			border-radius: 8px 0 0 8px;
			outline: none;
			box-shadow: 0 2px 6px rgba(0,0,0,0.1);
			transition: all 0.3s ease;
		}

		#search-input {
			flex: 1;
			padding: 0 20px;
			font-size: 16px;
			border: none;
			border-radius: 8px 0 0 8px;
			outline: none;
			box-shadow: 0 2px 6px rgba(0,0,0,0.1);
			transition: all 0.3s ease;
			height: 100%;
		}

		#search-button {
			padding: 0 25px;
			background-color: #0066ff;
			border: none;
			border-radius: 0 8px 8px 0;
			cursor: pointer;
			transition: all 0.3s ease;
			height: 100%;
			display: flex;
			align-items: center;
			justify-content: center;
		}

		#search-button:hover {
			background-color: #0052cc;
			transform: translateY(-1px);
		}

		#search-button svg {
			width: 24px;
			height: 24px;
		}

		.tips {
			color: rgba(255,255,255,0.8);
			margin-top: 20px;
			font-size: 0.9em;
		}

		@media (max-width: 480px) {
			.container {
				padding: 0 15px;
				min-height: 60vh;
			}

			.search-container {
				height: 45px;
			}
			
			#search-input {
				padding: 0 15px;
			}
			
			#search-button {
				padding: 0 20px;
			}
		}
		</style>
	</head>
	<body>
		<div class="container">
			<div class="logo">
				<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 18" fill="#ffffff" width="120" height="90">
					<path d="M23.763 6.886c-.065-.053-.673-.512-1.954-.512-.32 0-.659.03-1.01.087-.248-1.703-1.651-2.533-1.716-2.57l-.345-.2-.227.328a4.596 4.596 0 0 0-.611 1.433c-.23.972-.09 1.884.403 2.666-.596.331-1.546.418-1.744.42H.752a.753.753 0 0 0-.75.749c-.007 1.456.233 2.864.692 4.07.545 1.43 1.355 2.483 2.409 3.13 1.181.725 3.104 1.14 5.276 1.14 1.016 0 2.03-.092 2.93-.266 1.417-.273 2.705-.742 3.826-1.391a10.497 10.497 0 0 0 2.61-2.14c1.252-1.42 1.998-3.005 2.553-4.408.075.003.148.005.221.005 1.371 0 2.215-.55 2.68-1.01.505-.5.685-.998.704-1.053L24 7.076l-.237-.19Z"></path>
					<path d="M2.216 8.075h2.119a.186.186 0 0 0 .185-.186V6a.186.186 0 0 0-.185-.186H2.216A.186.186 0 0 0 2.031 6v1.89c0 .103.083.186.185.186Zm2.92 0h2.118a.185.185 0 0 0 .185-.186V6a.185.185 0 0 0-.185-.186H5.136A.185.185 0 0 0 4.95 6v1.89c0 .103.083.186.186.186Zm2.964 0h2.118a.186.186 0 0 0 .185-.186V6a.186.186 0 0 0-.185-.186H8.1A.185.185 0 0 0 7.914 6v1.89c0 .103.083.186.186.186Zm2.928 0h2.119a.185.185 0 0 0 .185-.186V6a.185.185 0 0 0-.185-.186h-2.119a.186.186 0 0 0-.185.186v1.89c0 .103.083.186.185.186Zm-5.892-2.72h2.118a.185.185 0 0 0 .185-.186V3.28a.186.186 0 0 0-.185-.186H5.136a.186.186 0 0 0-.186.186v1.89c0 .103.083.186.186.186Zm2.964 0h2.118a.186.186 0 0 0 .185-.186V3.28a.186.186 0 0 0-.185-.186H8.1a.186.186 0 0 0-.186.186v1.89c0 .103.083.186.186.186Zm2.928 0h2.119a.185.185 0 0 0 .185-.186V3.28a.186.186 0 0 0-.185-.186h-2.119a.186.186 0 0 0-.185.186v1.89c0 .103.083.186.185.186Zm0-2.72h2.119a.186.186 0 0 0 .185-.186V.56a.185.185 0 0 0-.185-.186h-2.119a.186.186 0 0 0-.185.186v1.89c0 .103.083.186.185.186Zm2.955 5.44h2.118a.185.185 0 0 0 .186-.186V6a.185.185 0 0 0-.186-.186h-2.118a.185.185 0 0 0-.185.186v1.89c0 .103.083.186.185.186Z"></path>
				</svg>
			</div>
			<div class="search-container">
				<input type="text" id="search-input" placeholder="请输入镜像关键词">
				<button id="search-button" title="搜索">
					<svg focusable="false" aria-hidden="true" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
						<path d="M21 21L16.65 16.65M19 11C19 15.4183 15.4183 19 11 19C6.58172 19 3 15.4183 3 11C3 6.58172 6.58172 3 11 3C15.4183 3 19 6.58172 19 11Z" stroke="white" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"></path>
					</svg>
				</button>
			</div>
		</div>
		<script>
		function performSearch() {
			const query = document.getElementById('search-input').value;
			if (query) {
				window.location.href = '/search?q=' + encodeURIComponent(query);
			}
		}
	
		document.getElementById('search-button').addEventListener('click', performSearch);
		document.getElementById('search-input').addEventListener('keypress', function(event) {
			if (event.key === 'Enter') {
				performSearch();
			}
		});
		</script>
	</body>
	</html>`
}

// newUrl 构造一个新的 URL 对象，基于给定 base
func newUrl(urlStr, base string) *url.URL {
	baseURL, err := url.Parse(base)
	if err != nil {
		log.Printf("解析 base URL 错误: %v", err)
		return nil
	}
	u, err := baseURL.Parse(urlStr)
	if err != nil {
		log.Printf("构造新 URL 错误: %v", err)
		return nil
	}
	return u
}

// anyContains 判断目标字符串 s 是否包含切片中任一子串
func anyContains(s string, substrs []string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// copyHeader 将 src 中的所有 header 复制到 dst 中
func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// copyResponse 将响应 resp 的 header 与 body 写入到 http.ResponseWriter
func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// processResponseHeaders 修改响应中 Www-Authenticate 头（将认证地址替换为 workersUrl）
func processResponseHeaders(resp *http.Response, workersUrl string) {
	if wwwAuth := resp.Header.Get("Www-Authenticate"); wwwAuth != "" {
		newAuth := strings.ReplaceAll(wwwAuth, authURL, workersUrl)
		resp.Header.Set("Www-Authenticate", newAuth)
	}
}

// proxy 发起代理请求并对响应做一些头部调整
func proxy(w http.ResponseWriter, req *http.Request, rawLen string) {
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if rawLen != "" {
		newLen := resp.Header.Get("content-length")
		if rawLen != newLen {
			w.Header().Set("--error", "bad len: "+newLen+", expect: "+rawLen)
			w.Header().Set("access-control-expose-headers", "--error")
			http.Error(w, "bad length", http.StatusBadRequest)
			return
		}
	}
	// 设置 CORS 及缓存头
	resp.Header.Set("access-control-expose-headers", "*")
	resp.Header.Set("access-control-allow-origin", "*")
	resp.Header.Set("Cache-Control", "max-age=1500")
	// 删除安全相关头
	resp.Header.Del("content-security-policy")
	resp.Header.Del("content-security-policy-report-only")
	resp.Header.Del("clear-site-data")

	copyResponse(w, resp)
}

// fixURL 对 URL 进行编码修正
func fixURL(u *url.URL) *url.URL {
	if !strings.Contains(u.RawQuery, "%2F") && strings.Contains(u.String(), "%3A") {
		modifiedUrlStr := strings.Replace(u.String(), "%3A", "%3Alibrary%2F", 1)
		newURL, err := url.Parse(modifiedUrlStr)
		if err == nil {
			return newURL
		}
	}
	return u
}

// cloneBody 返回一个新的 io.ReadCloser，用于复制请求体
func cloneBody(body []byte) io.ReadCloser {
	if len(body) == 0 {
		return nil
	}
	return io.NopCloser(bytes.NewReader(body))
}

// adjustAcceptHeader 确保 Accept 头包含对 OCI index 的支持
func adjustAcceptHeader(header http.Header) {
	accept := header.Get("Accept")
	if accept == "" {
		accept = "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json"
	} else if !strings.Contains(accept, "application/vnd.oci.image.index.v1+json") {
		accept = accept + ", application/vnd.oci.image.index.v1+json"
	}
	header.Set("Accept", accept)
}

// httpHandler 用于处理重定向和 OPTIONS（预检）请求，新增 body 参数以支持重放请求体
func httpHandler(w http.ResponseWriter, r *http.Request, pathname, baseHost string, body []byte) {
	// 处理预检请求
	if r.Method == "OPTIONS" && r.Header.Get("access-control-request-headers") != "" {
		w.Header().Set("access-control-allow-origin", "*")
		w.Header().Set("access-control-allow-methods", "GET,POST,PUT,PATCH,TRACE,DELETE,HEAD,OPTIONS")
		w.Header().Set("access-control-max-age", "1728000")
		w.WriteHeader(http.StatusOK)
		return
	}

	newHeaders := r.Header.Clone()
	newHeaders.Del("Authorization")
	adjustAcceptHeader(newHeaders)

	urlObj := newUrl(pathname, "https://"+baseHost)
	if urlObj == nil {
		http.Error(w, "Invalid URL", http.StatusBadRequest)
		return
	}
	proxyReq, err := http.NewRequest(r.Method, urlObj.String(), cloneBody(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	proxyReq.Header = newHeaders
	proxy(w, proxyReq, "")
}

func mainHandler(w http.ResponseWriter, r *http.Request) {
	// 读取并缓存请求体，便于重用
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusInternalServerError)
			return
		}
	}

	workersUrl := "https://" + r.Host
	query := r.URL.Query()
	ns := query.Get("ns")
	hostname := query.Get("hubhost")
	if hostname == "" {
		hostname = r.Host
	}
	hostParts := strings.Split(hostname, ".")
	hostTop := hostParts[0]

	var localHubHost string
	fakePage := false
	if ns != "" {
		if ns == "docker.io" {
			localHubHost = "registry-1.docker.io"
		} else {
			localHubHost = ns
		}
	} else {
		var rp bool
		localHubHost, rp = routeByHosts(hostTop)
		fakePage = rp
	}

	log.Printf("域名头部: %s 反代地址: %s searchInterface: %v", hostTop, localHubHost, fakePage)

	// 修改请求 URL 的 scheme 与 host
	r.URL.Scheme = "https"
	r.URL.Host = localHubHost

	hubParams := []string{"/v1/search", "/v1/repositories"}
	userAgent := strings.ToLower(r.Header.Get("User-Agent"))

	// 针对包含 mozilla 或指定路径的请求走页面/转发逻辑
	if strings.Contains(userAgent, "mozilla") || anyContains(r.URL.Path, hubParams) {
		if r.URL.Path == "/" {
			if url302 := os.Getenv("URL302"); url302 != "" {
				http.Redirect(w, r, url302, http.StatusFound)
				return
			} else if urlEnv := os.Getenv("URL"); urlEnv != "" {
				proxyReq, err := http.NewRequest(r.Method, urlEnv, cloneBody(bodyBytes))
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				copyHeader(proxyReq.Header, r.Header)
				adjustAcceptHeader(proxyReq.Header)
				resp, err := client.Do(proxyReq)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return
				}
				defer resp.Body.Close()
				copyResponse(w, resp)
				return
			} else {
				if fakePage {
					w.Header().Set("Content-Type", "text/html; charset=UTF-8")
					io.WriteString(w, searchInterface())
					return
				}
			}
		} else {
			if fakePage {
				// fakePage 模式下，将 host 改为 hub.docker.com
				r.URL.Host = "hub.docker.com"
			}
			// 若查询参数 q 中包含 "library/" 但不等于 "library/" 则去除之
			if q := query.Get("q"); strings.Contains(q, "library/") && q != "library/" {
				q = strings.Replace(q, "library/", "", 1)
				query.Set("q", q)
				r.URL.RawQuery = query.Encode()
			}
			newReq, err := http.NewRequest(r.Method, r.URL.String(), cloneBody(bodyBytes))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			copyHeader(newReq.Header, r.Header)
			adjustAcceptHeader(newReq.Header)
			resp, err := client.Do(newReq)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			processResponseHeaders(resp, workersUrl)
			if loc := resp.Header.Get("Location"); loc != "" {
				httpHandler(w, r, loc, localHubHost, bodyBytes)
				return
			}
			copyResponse(w, resp)
			return
		}
	}

	// 修正 URL 编码问题
	r.URL = fixURL(r.URL)

	// 处理 token 请求
	if strings.Contains(r.URL.Path, "/token") {
		tokenURL := authURL + r.URL.Path + "?" + r.URL.RawQuery
		tokenReq, err := http.NewRequest(r.Method, tokenURL, cloneBody(bodyBytes))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		tokenReq.Header.Set("Host", "auth.docker.io")
		copyHeader(tokenReq.Header, r.Header)
		adjustAcceptHeader(tokenReq.Header)
		tokenReq.Header.Set("Connection", "keep-alive")
		tokenReq.Header.Set("Cache-Control", "max-age=0")
		resp, err := client.Do(tokenReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		copyResponse(w, resp)
		return
	}

	// 针对 registry-1.docker.io 特定请求修改路径前缀
	if localHubHost == "registry-1.docker.io" {
		matched, _ := regexp.MatchString(`^/v2/[^/]+/[^/]+/[^/]+$`, r.URL.Path)
		if matched && !strings.HasPrefix(r.URL.Path, "/v2/library") {
			parts := strings.SplitN(r.URL.Path, "/v2/", 2)
			if len(parts) == 2 {
				r.URL.Path = "/v2/library/" + parts[1]
			}
			log.Printf("modified_url: %s", r.URL.Path)
		}
	}

	newReq, err := http.NewRequest(r.Method, r.URL.String(), cloneBody(bodyBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newReq.Header.Set("Host", localHubHost)
	newReq.Header.Set("User-Agent", r.Header.Get("User-Agent"))
	// 处理 Accept 头，确保 OCI index 类型被包含
	{
		accept := r.Header.Get("Accept")
		if accept == "" {
			accept = "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json"
		} else if !strings.Contains(accept, "application/vnd.oci.image.index.v1+json") {
			accept = accept + ", application/vnd.oci.image.index.v1+json"
		}
		newReq.Header.Set("Accept", accept)
	}
	newReq.Header.Set("Accept-Language", r.Header.Get("Accept-Language"))
	newReq.Header.Set("Accept-Encoding", r.Header.Get("Accept-Encoding"))
	newReq.Header.Set("Connection", "keep-alive")
	newReq.Header.Set("Cache-Control", "max-age=0")
	if auth := r.Header.Get("Authorization"); auth != "" {
		newReq.Header.Set("Authorization", auth)
	}
	if xAmz := r.Header.Get("X-Amz-Content-Sha256"); xAmz != "" {
		newReq.Header.Set("X-Amz-Content-Sha256", xAmz)
	}

	resp, err := client.Do(newReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	processResponseHeaders(resp, workersUrl)
	if loc := resp.Header.Get("Location"); loc != "" {
		httpHandler(w, r, loc, localHubHost, bodyBytes)
		return
	}
	copyResponse(w, resp)
}

func main() {
	http.HandleFunc("/", mainHandler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "50001"
	}
	log.Printf("Listening on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

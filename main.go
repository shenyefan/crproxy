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

// routeByHosts 根据主机名选择对应的上游地址
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

// newUrl 构造新的 URL 对象
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

// copyHeader 将 src 中所有 header 复制到 dst
func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// copyResponse 将响应 resp 的 header 和 body 写入到 ResponseWriter
func copyResponse(w http.ResponseWriter, resp *http.Response) {
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// processResponseHeaders 修改响应中 Www-Authenticate 头，将认证地址替换为 workersUrl
func processResponseHeaders(resp *http.Response, workersUrl string) {
	if wwwAuth := resp.Header.Get("Www-Authenticate"); wwwAuth != "" {
		newAuth := strings.ReplaceAll(wwwAuth, authURL, workersUrl)
		resp.Header.Set("Www-Authenticate", newAuth)
	}
}

// proxy 发起代理请求并调整部分 header
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

// fixURL 修正 URL 编码问题
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

// adjustAcceptHeader 确保 Accept 头支持 OCI 索引格式
func adjustAcceptHeader(header http.Header) {
	// 直接设置 Accept 头，确保包含需要的 MIME 类型
	header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json")
}

// httpHandler 用于处理重定向请求
func httpHandler(w http.ResponseWriter, r *http.Request, loc string, localHubHost string, bodyBytes []byte) {
	newURL, err := url.Parse(loc)
	if err != nil {
		http.Error(w, "无效的重定向 URL", http.StatusBadGateway)
		return
	}
	newReq, err := http.NewRequest(r.Method, newURL.String(), cloneBody(bodyBytes))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	newReq.Header.Set("Host", localHubHost)
	newReq.Header.Set("User-Agent", r.Header.Get("User-Agent"))
	adjustAcceptHeader(newReq.Header)
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
	processResponseHeaders(resp, "https://"+r.Host)
	if nextLoc := resp.Header.Get("Location"); nextLoc != "" {
		httpHandler(w, r, nextLoc, localHubHost, bodyBytes)
		return
	}
	copyResponse(w, resp)
}

// mainHandler 纯代理所有请求
func mainHandler(w http.ResponseWriter, r *http.Request) {
	// 读取请求体以便重放
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "读取请求体失败", http.StatusInternalServerError)
			return
		}
	}

	workersUrl := "https://" + r.Host
	query := r.URL.Query()
	// 读取 ns 参数，并在后续替换 Docker Hub 的默认命名空间（默认使用 library）
	ns := query.Get("ns")
	hostname := query.Get("hubhost")
	if hostname == "" {
		hostname = r.Host
	}
	hostParts := strings.Split(hostname, ".")
	hostTop := hostParts[0]

	localHubHost, _ := routeByHosts(hostTop)
	log.Printf("域名头部: %s 反代地址: %s", hostTop, localHubHost)

	r.URL.Scheme = "https"
	r.URL.Host = localHubHost
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

	// 使用 ns 参数替换默认的命名空间（默认为 library）
	nsToUse := ns
	if nsToUse == "" {
		nsToUse = "library"
	}

	// 针对 registry-1.docker.io 特定请求修改路径前缀
	if localHubHost == "registry-1.docker.io" {
		matched, _ := regexp.MatchString(`^/v2/[^/]+/[^/]+/[^/]+$`, r.URL.Path)
		if matched && !strings.HasPrefix(r.URL.Path, "/v2/"+nsToUse) {
			parts := strings.SplitN(r.URL.Path, "/v2/", 2)
			if len(parts) == 2 {
				r.URL.Path = "/v2/" + nsToUse + "/" + parts[1]
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
	adjustAcceptHeader(newReq.Header)
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
	// 若有重定向，则递归处理
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

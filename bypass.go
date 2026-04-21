package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	v8go "rogchap.com/v8go"
)

type CloudflareBypass struct {
	client *http.Client
	iso    *v8go.Isolate
}

func NewCloudflareBypass() *CloudflareBypass {
	// Cookie destekli client oluştur
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Redirect'leri otomatik takip etme
		},
	}

	iso := v8go.NewIsolate()
	return &CloudflareBypass{
		client: client,
		iso:    iso,
	}
}

func (cb *CloudflareBypass) getDomainFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}

func (cb *CloudflareBypass) solveChallenge(html, baseURL string) (string, error) {
	// Cloudflare challenge parametrelerini çıkar
	jschlVc := regexp.MustCompile(`name="jschl_vc" value="([^"]+)"`)
	pass := regexp.MustCompile(`name="pass" value="([^"]+)"`)

	// JavaScript challenge kodunu bul (daha kapsamlı regex)
	jschlAnswer := regexp.MustCompile(`setTimeout\(function\(\)\{(?:.*?;){2,}([^}]*)a\.value = (.+?);`)

	matchesVc := jschlVc.FindStringSubmatch(html)
	matchesPass := pass.FindStringSubmatch(html)
	matchesAnswer := jschlAnswer.FindStringSubmatch(html)

	if len(matchesVc) < 2 || len(matchesPass) < 2 {
		return "", fmt.Errorf("challenge parametreleri bulunamadı")
	}

	jschl_vc := matchesVc[1]
	passVal := matchesPass[1]

	// JavaScript kodunu temizle
	var jsScript string
	if len(matchesAnswer) >= 3 {
		jsScript = matchesAnswer[2]
	} else {
		// Alternatif yöntem: tüm script tag'ini bul
		scriptPattern := `setTimeout\(function[^}]+(var s,t,o,p,b,r,e,a,k,i,n,g,f[^}]+}a\.value = [^;]+);`
		scriptRe := regexp.MustCompile(scriptPattern)
		scriptMatches := scriptRe.FindStringSubmatch(html)
		if len(scriptMatches) >= 2 {
			jsScript = scriptMatches[1]
		} else {
			return "", fmt.Errorf("javascript challenge kodu bulunamadı")
		}
	}

	// JavaScript context oluştur
	ctx := v8go.NewContext(cb.iso)
	defer ctx.Close()

	// Cloudflare için gerekli JavaScript ortamını hazırla
	domain := cb.getDomainFromURL(baseURL)
	setupScript := fmt.Sprintf(`
		var window = {
			location: {
				hostname: "%s"
			}
		};
		var document = {
			getElementById: function(id) {
				return {
					innerHTML: ""
				};
			}
		};
	`, domain)

	_, err := ctx.RunScript(setupScript, "setup.js")
	if err != nil {
		return "", fmt.Errorf("javascript setup hatası: %v", err)
	}

	// Challenge script'ini çalıştır
	fullScript := jsScript + "; a.value;"
	result, err := ctx.RunScript(fullScript, "challenge.js")
	if err != nil {
		return "", fmt.Errorf("javascript çalıştırma hatası: %v", err)
	}

	answer := result.String()
	return fmt.Sprintf("jschl_vc=%s&pass=%s&jschl_answer=%s", jschl_vc, passVal, answer), nil
}

func (cb *CloudflareBypass) Bypass(rawURL string) (*http.Response, error) {
	fmt.Printf("Cloudflare bypass deneniyor: %s\n", rawURL)

	// İlk istek
	req, _ := http.NewRequest("GET", rawURL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	//req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Cache-Control", "max-age=0")

	resp, err := cb.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ilk istek hatası: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	html := string(body)

	fmt.Printf("İlk response status: %s\n", resp.Status)
	fmt.Printf("İlk response uzunluğu: %d\n", len(html))

	// Cloudflare challenge kontrolü
	if strings.Contains(html, "Checking your browser") &&
		strings.Contains(html, "jschl_vc") {

		fmt.Println("Cloudflare challenge tespit edildi, çözüm deneniyor...")

		// Challenge'ı çöz
		formData, err := cb.solveChallenge(html, rawURL)
		if err != nil {
			return nil, fmt.Errorf("challenge çözüm hatası: %v", err)
		}

		fmt.Printf("Çözülen form data: %s\n", formData[:50]+"...")

		// Cloudflare bekleme süresi (önemli!)
		fmt.Println("Cloudflare bekleme süresi (5 saniye)...")
		time.Sleep(5 * time.Second)

		// Çözümü gönder
		parsedURL, _ := url.Parse(rawURL)
		submitURL := fmt.Sprintf("https://%s/cdn-cgi/l/chk_jschl?%s", parsedURL.Host, formData)

		fmt.Printf("Challenge çözümü gönderiliyor: %s\n", submitURL)

		submitReq, _ := http.NewRequest("GET", submitURL, nil)
		submitReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
		submitReq.Header.Set("Referer", rawURL)
		submitReq.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		submitReq.Header.Set("Accept-Language", "en-US,en;q=0.9")
		submitReq.Header.Set("Connection", "keep-alive")
		submitReq.Header.Set("Upgrade-Insecure-Requests", "1")

		submitResp, err := cb.client.Do(submitReq)
		if err != nil {
			return nil, fmt.Errorf("challenge çözüm isteği hatası: %v", err)
		}

		fmt.Printf("Challenge çözüm response status: %s\n", submitResp.Status)

		// Eğer redirect geldiyse, orijinal URL'e yönlendir
		if submitResp.StatusCode == 302 || submitResp.StatusCode == 301 {
			location := submitResp.Header.Get("Location")
			if location != "" {
				fmt.Printf("Redirect tespit edildi: %s\n", location)
				redirectReq, _ := http.NewRequest("GET", location, nil)
				redirectReq.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				redirectReq.Header.Set("Referer", submitURL)
				return cb.client.Do(redirectReq)
			}
		}

		return submitResp, nil
	}

	// Challenge yoksa doğrudan response döndür
	fmt.Println("Cloudflare challenge bulunamadı, doğrudan response dönüyor")
	return &http.Response{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Proto:         resp.Proto,
		ProtoMajor:    resp.ProtoMajor,
		ProtoMinor:    resp.ProtoMinor,
		Header:        resp.Header,
		Body:          io.NopCloser(strings.NewReader(html)),
		ContentLength: int64(len(html)),
	}, nil
}

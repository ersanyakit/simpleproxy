package main

import (
	"context"
	"encoding/json"
	"fmt"
	"generator/helpers"
	"html"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/quic-go/quic-go/http3"

	utls "github.com/refraction-networking/utls"
)

func generateURL(username string) string {
	var domains = []string{
		"https://chaturbate.com/%s/",
		"https://de.chaturbate.com/%s/",
		"https://en.chaturbate.com/%s/",
		"https://es.chaturbate.com/%s/",
		"https://fr.chaturbate.com/%s/",
		"https://it.chaturbate.com/%s/",
		"https://ja.chaturbate.com/%s/",
		"https://ko.chaturbate.com/%s/",
		"https://pl.chaturbate.com/%s/",
		"https://pt.chaturbate.com/%s/",
		"https://ru.chaturbate.com/%s/",
		"https://zh-hans.chaturbate.com/%s/",
		"https://zh-hant.chaturbate.com/%s/",
	}
	domain := domains[rand.Intn(len(domains))]
	return fmt.Sprintf(domain, username)
}

var re = regexp.MustCompile(`window\.initialRoomDossier\s*=\s*"(.*?)";`)

func ParseRoomDossierFromHTML(htmlStr string) (map[string]any, error) {

	matches := re.FindStringSubmatch(htmlStr)
	if len(matches) < 2 {
		return nil, fmt.Errorf("room dossier not found")
	}
	raw := matches[1]
	raw = html.UnescapeString(raw)

	decoded, err := strconv.Unquote(`"` + raw + `"`)

	if err != nil {

		return nil, err

	}
	decoded = strings.Trim(decoded, "\"")
	var result map[string]any
	if err := json.Unmarshal([]byte(decoded), &result); err != nil {
		return nil, err
	}
	return result, nil

}

func fetchLegacyEx(username string) (url string, err error) {
	client := &http.Client{
		Transport: &http3.Transport{},
	}

	domain := generateURL(username)
	resp, err := client.Get(domain)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	result, err := ParseRoomDossierFromHTML(string(body))
	if err != nil {
		log.Fatal(err)
	}

	hlsSource, ok := result["hls_source"].(string)
	if !ok {
		return "", fmt.Errorf("hls_source is not string")
	}
	return hlsSource, nil

}

func fetch(username string) (string, error) {
	jar, _ := cookiejar.New(nil)

	client := &http.Client{
		Transport: &http3.Transport{},
		Timeout:   15 * time.Second,
		Jar:       jar,
	}

	domain := generateURL(username)

	data := url.Values{}

	data.Set("room_slug", username)
	data.Set("bandwidth", "high")
	data.Set("current_edge", "")
	data.Set("exclude_edge", "")

	domain = generateURL("get_edge_hls_url_ajax")
	req, err := http.NewRequest(
		"POST",
		domain,
		strings.NewReader(data.Encode()))
	if err != nil {
		fmt.Println("ERR", err)
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", helpers.RandomUserAgent())
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "tr,en-US;q=0.9,en;q=0.8,ja;q=0.7,zh-TW;q=0.6,zh-CN;q=0.5,zh;q=0.4,th;q=0.3")

	req.Header.Set("Referer", "https://chaturbate.com/"+username+"/")
	req.Header.Set("Origin", "https://chaturbate.com/"+username+"/")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Println("ERR", err)

		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	return string(body), nil
}

func fetchQUIC(username string) (string, error) {
	jar, _ := cookiejar.New(nil)

	client := &http.Client{
		Transport: &http3.Transport{},
		Timeout:   15 * time.Second,
		Jar:       jar,
	}

	// seed request (cookie almak için)
	seedURL := generateURL(username)
	_, _ = client.Get(seedURL)

	data := url.Values{}
	data.Set("room_slug", username)
	data.Set("bandwidth", "high")
	data.Set("current_edge", "")
	data.Set("exclude_edge", "")

	req, err := http.NewRequest(
		"POST",
		"https://chaturbate.com/get_edge_hls_url_ajax/",
		strings.NewReader(data.Encode()),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", helpers.RandomUserAgent())
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Origin", "https://chaturbate.com")
	req.Header.Set("Referer", seedURL)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	return string(body), nil
}

func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)

	tr := &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := net.DialTimeout(network, addr, 10*time.Second)
			if err != nil {
				return nil, err
			}

			host := addr
			if strings.Contains(addr, ":") {
				host = strings.Split(addr, ":")[0]
			}

			config := &utls.Config{
				ServerName: host,
			}

			// Chrome TLS fingerprint
			uconn := utls.UClient(conn, config, utls.HelloChrome_Auto)

			if err := uconn.Handshake(); err != nil {
				return nil, err
			}

			return uconn, nil
		},
	}

	return &http.Client{
		Transport: tr,
		Jar:       jar,
		Timeout:   20 * time.Second,
	}
}

func fetchLegacy(username string, pool *Pool) (string, error) {
	url := generateURL(username)

	html, err := pool.Fetch(url)
	if err != nil {
		return "", err
	}

	result, err := ParseRoomDossierFromHTML(html)
	if err != nil {
		return "", err
	}

	hls, ok := result["hls_source"].(string)
	if !ok {
		return "", fmt.Errorf("not found")
	}

	return hls, nil
}

func main() {

	app := fiber.New(fiber.Config{
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  0,
	})

	pool := NewPool(5) // aynı anda 5 browser worker

	app.Get("/online", func(c fiber.Ctx) error {
		all, err := fetchAll()
		if err != nil {
			fmt.Println("error:", err)
			return err
		}
		return c.JSON(all)
	})

	app.Get("/:username", func(c fiber.Ctx) error {
		username := fiber.Params[string](c, "username")

		json, jsonerr := fetch(username)
		if jsonerr != nil {
			return c.JSON(fiber.Map{
				"success": false,
				"data":    "",
			})
		}

		return c.SendString(json)
	})

	app.Get("/quic/:username", func(c fiber.Ctx) error {
		username := fiber.Params[string](c, "username")
		json, jsonerr := fetchQUIC(username)
		if jsonerr != nil {
			return c.JSON(fiber.Map{
				"success": false,
				"data":    "",
			})
		}

		return c.SendString(json)
	})

	app.Get("/source/:username", func(c fiber.Ctx) error {
		username := fiber.Params[string](c, "username")
		json, jsonerr := fetchLegacy(username, pool)
		if jsonerr != nil {
			return c.JSON(fiber.Map{
				"success": false,
				"data":    "",
			})
		}

		return c.JSON(fiber.Map{
			"success": true,
			"data":    json,
		})
	})

	log.Fatal(app.Listen(":3000"))

}

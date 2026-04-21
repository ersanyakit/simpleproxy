package main

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"log"
	"math/rand"
	"regexp"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v3"
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

	bypass := NewCloudflareBypass()
	domain := generateURL(username)

	resp, err := bypass.Bypass(domain)
	if err != nil {
		fmt.Printf("Hata: %v\n", err)
		return "", err

	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	result, err := ParseRoomDossierFromHTML(string(body))
	if err != nil {
		return "", err
	}

	hlsSource, ok := result["hls_source"].(string)
	if !ok {

		return "", fmt.Errorf("hls_source not found in map")

	}

	return hlsSource, nil

}

func main() {

	app := fiber.New(fiber.Config{
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  0,
	})

	app.Get("/online", func(c fiber.Ctx) error {
		all, err := fetchAll()
		if err != nil {
			fmt.Println("error:", err)
			return err
		}
		return c.JSON(all)
	})

	app.Get("/source/:username", func(c fiber.Ctx) error {
		username := fiber.Params[string](c, "username")
		json, jsonerr := fetchLegacyEx(username)
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

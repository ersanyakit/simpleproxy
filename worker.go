package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"generator/helpers"
	"io"
	"net/http"

	"github.com/quic-go/quic-go/http3"
)

type PageResult struct {
	Offset int
	Data   []Room
	Err    error
}

type Room struct {
	DisplayAge    int      `json:"display_age"`
	Gender        string   `json:"gender"`
	Location      string   `json:"location"`
	CurrentShow   string   `json:"current_show"`
	Username      string   `json:"username"`
	RoomSubject   string   `json:"room_subject"`
	Tags          []string `json:"tags"`
	NumUsers      int      `json:"num_users"`
	NumFollowers  int      `json:"num_followers"`
	StartDTUTC    string   `json:"start_dt_utc"`
	Country       string   `json:"country"`
	HasPassword   bool     `json:"has_password"`
	PrivatePrice  int      `json:"private_price"`
	SpyShowPrice  int      `json:"spy_show_price"`
	IsAgeVerified bool     `json:"is_age_verified"`
	Label         string   `json:"label"`
	Img           string   `json:"img"`
	Subject       string   `json:"subject"`
}

type RoomListResponse struct {
	Rooms         []Room `json:"rooms"`
	TotalCount    int    `json:"total_count"`
	AllRoomsCount int    `json:"all_rooms_count"`
	RoomListID    string `json:"room_list_id"`
}

func newHTTP3Client() *http.Client {
	return &http.Client{
		Timeout: 0,
		Transport: &http3.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			// QUIC fallback
			DisableCompression: false,
		},
	}
}

func fetchURL(url string) (*RoomListResponse, error) {
	client := newHTTP3Client()

	fmt.Println("URL", url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", helpers.RandomUserAgent())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	fmt.Println("GELEN", string(body))

	var result RoomListResponse
	if err := json.Unmarshal(body, &result); err != nil {

		return nil, err
	}

	return &result, nil
}

func fetchAll() ([]Room, error) {
	limit := 100
	total := 6218

	workers := 8

	jobs := make(chan int, workers)
	results := make(chan PageResult, workers)

	for i := 0; i < workers; i++ {
		go func() {
			for offset := range jobs {
				url := fmt.Sprintf(
					"https://chaturbate.com/api/ts/roomlist/room-list/?limit=%d&offset=%d",
					limit,
					offset,
				)

				data, err := fetchURL(url)

				fmt.Println("DATA", data)
				var rooms []Room
				if err == nil && data != nil {
					rooms = data.Rooms
				}

				results <- PageResult{
					Offset: offset,
					Data:   rooms,
					Err:    err,
				}
			}
		}()
	}

	go func() {
		for offset := 0; offset < total; offset += limit {
			jobs <- offset
		}
		close(jobs)
	}()

	expected := (total + limit - 1) / limit

	var all []Room

	for i := 0; i < expected; i++ {
		res := <-results
		if res.Err != nil {
			fmt.Println("error:", res.Offset, res.Err)
			continue
		}
		all = append(all, res.Data...)
	}

	return all, nil
}

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/koron/go-dproxy"
)

type tomlConfig struct {
	URL   string `toml:"discord_webhook_url"`
	Users map[string]user
}

type user struct {
	GithubID  string `toml:"github_id"`
	DiscordID string `toml:"discord_id"`
}

type discordEmbed struct {
	UserName  string   `json:"username"`
	AvatarURL string   `json:"avatar_url"`
	Content   string   `json:"content"`
	Embeds    []embeds `json:"embeds"`
}

type embeds struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Color       int64  `json:"color"` //2829099
	Footer      footer `json:"footer"`
	// Image image `json:"image"`
	Thumbnail image `json:"thumbnail"`
}

type footer struct {
	Text string `json:"text"`
	// IconURL string `json:"icon_url"`
}

type image struct {
	URL string `json:"url"`
}

var re = regexp.MustCompile(`\B@[^ \t\n\r\f]+`)
var config tomlConfig

func main() {
	if _, err := toml.DecodeFile("settings.toml", &config); err != nil {
		fmt.Println(err)
		return
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	http.HandleFunc("/", handler)
	_ = http.ListenAndServe(fmt.Sprintf(":%s", port), nil)

}

func handler(w http.ResponseWriter, r *http.Request) {
	// Please POST me
	if r.Method != "POST" {
		fmt.Println("request is not POST")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Please send me json
	if r.Header.Get("Content-Type") != "application/json" {
		fmt.Println("request doesn't have 'Content-Type'")
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Read body
	body, err := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	// fmt.Println(string(body))

	var v interface{}
	err = json.Unmarshal(body, &v)
	if err != nil {
		fmt.Println(err)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("failed to parse request body to json"))
		return
	}
	log.Println(string(body))

	// created events only supported
	action, _ := dproxy.New(v).M("action").String()
	if action != "created" {
		fmt.Println(action, "is not created events")
		w.WriteHeader(http.StatusOK)
		return
	}

	comment, err := dproxy.New(v).M("comment").M("body").String()
	if err != nil {
		fmt.Println("comment field not found")
		w.WriteHeader(http.StatusOK)
		return
	}

	mentions := parseMention(comment)
	if len(mentions) == 0 {
		fmt.Println("No one mentioned")
		w.WriteHeader(http.StatusOK)
		return
	}
	fmt.Println("parsed", mentions)

	var discordIDs []string

	for _, githubIDPrefix := range mentions {
		discordID, err := getDiscordID(githubIDPrefix)
		if err != nil {
			fmt.Println(err)
			continue
		}
		discordIDs = append(discordIDs, "<@"+discordID+">さん")
	}

	var str strings.Builder
	str.WriteString("クルッポー\r")

	// is this a PR comment?
	_, err = dproxy.New(v).M("issue").M("pull_request").M("url").String()
	// is this a PR review comment?
	_, reviewError := dproxy.New(v).M("comment").M("pull_request_url").String()
	if err == nil || reviewError == nil {
		fmt.Println("this is PR comment")
		str.WriteString("ポッポー（PRでメンションされています。）\r")
	} else {
		fmt.Println(err)
		fmt.Println("this is issue comment")
		str.WriteString("ポッポー（Issueでメンションされています。）\r")
	}

	str.WriteString(strings.Join(discordIDs, " "))

	fmt.Println("メッセージ\r", str.String())

	url, _ := dproxy.New(v).M("comment").M("html_url").String()
	var title string
	// PR review comment
	if reviewError == nil {
		title, _ = dproxy.New(v).M("pull_request").M("title").String()
	} else {
		title, _ = dproxy.New(v).M("issue").M("title").String()
	}
	fmt.Println(url)
	timestamp, _ := dproxy.New(v).M("comment").M("created_at").String()
	avatarURL, _ := dproxy.New(v).M("comment").M("user").M("avatar_url").String()

	jsonBody := discordEmbed{
		UserName:  "伝書鳩",
		AvatarURL: "http://pancos-sozai.com/wp-content/uploads/%E3%83%8F%E3%83%88%E3%81%AE%E3%82%A4%E3%83%A9%E3%82%B9%E3%83%88%E7%B4%A0%E6%9D%9011.png",
		Content:   str.String(),
		Embeds: []embeds{
			{Title: title,
				Description: comment,
				URL:         url,
				Color:       2829099,
				Footer: footer{
					Text: timestamp,
				},
				Thumbnail: image{
					URL: avatarURL,
				}},
		},
	}
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	err = encoder.Encode(jsonBody)
	if err != nil {
		fmt.Println(err)
	}

	err = httpPost(config.URL, buf.Bytes())
	if err != nil {
		log.Fatal(err)
	}

	w.WriteHeader(http.StatusOK)
}

func parseMention(comment string) []string {
	result := re.FindAllString(comment, -1)

	return result
}

func getDiscordID(githubIDPrefix string) (string, error) {
	githubID := strings.TrimPrefix(githubIDPrefix, "@")
	for _, user := range config.Users {
		if user.GithubID == githubID {
			return user.DiscordID, nil
		}
	}
	return "", fmt.Errorf("%s not found", githubID)
}

func httpPost(url string, messageByte []byte) error {
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(messageByte))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return err
}

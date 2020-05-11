package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
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

var discordWebhook = "https://discordapp.com/api/webhooks/709079444673003552/7X6V2edOC3LTcIK0abKexltwhDeP6kbCGMQ42B-laqxcB8_cdVbxhzKKOqTZMWHLyUYQ"
var re = regexp.MustCompile(`\B@[^ \t\n\r\f]+`)
var config tomlConfig

func main() {
	if _, err := toml.DecodeFile("settings.toml", &config); err != nil {
		fmt.Println(err)
		return
	}

	http.HandleFunc("/", handler)
	_ = http.ListenAndServe(":3000", nil)

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
	str.WriteString(`クルッポー\r`)

	// is this a PR comment?
	_, err = dproxy.New(v).M("issue").M("pull_request").M("url").String()
	if err == nil {
		fmt.Println("this is PR comment")
		str.WriteString(`ポッポー（PRでメンションされています。）\r`)
	} else {
		fmt.Println(err)
		fmt.Println("this is issue comment")
		str.WriteString(`ポッポー（Issueでメンションされています。）\r`)
	}

	str.WriteString(strings.Join(discordIDs, " "))

	fmt.Println("メッセージ\r", str.String())

	err = httpPost(discordWebhook, str.String())
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

func httpPost(url, message string) error {
	json := `{"content":"` + message + `"}`
	fmt.Println(json)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer([]byte(json)))
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

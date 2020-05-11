package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	githubID  string `toml:"github_id"`
	discordID string `toml:"discord_id"`
}

var re = regexp.MustCompile(`\B@[^ \t\n\r\f]+`)
var config tomlConfig

func main() {
	if _, err := toml.Decode("settings.toml", &config); err != nil {
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

	// // Get length
	// length, err := strconv.Atoi(r.Header.Get("Content-Length"))
	// if err != nil {
	// 	fmt.Println("request doesn't have 'Content-Length'")
	// 	w.WriteHeader(http.StatusInternalServerError)
	// 	return
	// }

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

	var str strings.Builder
	str.WriteString("クルッポー")

	// is this a PR comment?
	isPr := !dproxy.New(v).M("created").M("issue").M("pull_request").Nil()
	if isPr {
		fmt.Println("this is PR comment")
		str.WriteString("PRでメンションされています。")
	} else {
		fmt.Println("this is issue comment")
		str.WriteString("Issueでメンションされています。")
	}

	// dump, _ := httputil.DumpRequest(r, true)
	// fmt.Println(string(dump))
	w.WriteHeader(http.StatusOK)
}

func parseMention(comment string) []string {
	result := re.FindAllString(comment, -1)

	return result
}

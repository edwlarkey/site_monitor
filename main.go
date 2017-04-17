package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	gomail "gopkg.in/gomail.v2"
	"gopkg.in/urfave/cli.v1"
)

// Config file structure
type Config struct {
	From        string
	LogFilePath string
	Wait        int
	Timeout     int
	Recipient   string
	Mailserver  string
	Username    string
	Password    string
	Port        int
	Sites       []string
}

// Site to check
type Site struct {
	URL    string
	Status int
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

var sites = make([]Site, 0)
var config Config

/*
 * sends email when site status changes
 */
func notify(status string, site *Site) {

	time := time.Now().Format("Mon Jan 02 15:04:05 2006")
	body := "<center><h3>" + site.URL + " is " + status + "</h3>"
	body += fmt.Sprintf("<p><strong>(%d)</strong> @ %s</p>", site.Status, time)
	body += "<br><br></center>"

	m := gomail.NewMessage()
	m.SetHeader("From", config.From)
	m.SetHeader("To", config.Recipient)
	m.SetHeader("Subject", "Monitor - "+site.URL+" - "+status)
	m.SetBody("text/html", body)

	d := gomail.NewDialer(config.Mailserver, config.Port, config.Username, config.Password)

	if err := d.DialAndSend(m); err != nil {
		panic(err)
	}
}

/*
 * recursive func that sends websites to the jobs queue.
 * runs every config.Wait seconds.
 */
func perform(jobs chan<- *Site) {
	for j := 0; j < len(sites); j++ {
		jobs <- &sites[j]
	}

	// for a := 0; a < len(sites); a++ {
	// 	<-results
	// }

	select {
	case <-time.After(time.Duration(config.Wait) * time.Second):
		perform(jobs)
	}
}

func writeLog(url string, status string) {
	switch status {
	case "UP":
		log.Println("INFO: ", url, "is", status)
	case "BACK UP":
		log.Println("NOTICE: ", url, "is", status)
	case "DOWN":
		log.Println("ALERT: ", url, "is", status)
	}
}

/*
 * goroutine that processes a website and reports its status.
 */
// func worker(id int, jobs <-chan *Site, results chan<- int) {
func worker(id int, jobs <-chan *Site) {

	for j := range jobs {
		status := check_http_status(j.URL, true)
		if status != j.Status {
			// status changed, lets notify
			state := "UP"
			if status <= 399 && j.Status > 399 {
				state = "BACK UP"
			}
			if status > 399 {
				state = "DOWN"
			}
			fmt.Println("[", id, "] - ", j.URL, "is ", state, " -", status)

			writeLog(j.URL, state)

			j.Status = status
			if state == "BACK UP" || state == "DOWN" {
				notify(state, j)
			}
		}
		j.Status = status
		// results <- j.Status
	}
}

/*
 * send GET request to url.
 * Returns status code.
 */
func check_http_status(url string, retry bool) int {
	status := 408 // Something went wrong, default to 'ClientRequestTimeout'
	timeout := time.Duration(config.Timeout) * time.Second
	client := http.Client{Timeout: timeout}

	res, err := client.Head(url)
	if err != nil {
		// fmt.Println("Unable to check")
		// fmt.Println(err)
		// if retry {
		// 	return check_http_status(url, false)
		// }
	}
	if res != nil {
		defer res.Body.Close()
		status = res.StatusCode
	}
	return status
}

func main() {
	var configFile string

	app := cli.NewApp()

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "config, c",
			Usage:       "Load configuration from `FILE`",
			Destination: &configFile,
		},
	}

	app.Action = func(c *cli.Context) error {
		filename, _ := filepath.Abs(configFile)

		jobs := make(chan *Site, 100)
		// results := make(chan int, 100)
		done := make(chan bool, 1)

		f, err := ioutil.ReadFile(filename)
		check(err)

		jsonErr := json.Unmarshal(f, &config)
		check(jsonErr)

		// open log file
		logf, logerr := os.OpenFile(config.LogFilePath, os.O_APPEND|os.O_CREATE|os.O_RDWR, 0777)
		if logerr != nil {
			fmt.Printf("error opening file: %v", logerr)
		}

		defer logf.Close()
		log.SetOutput(logf)

		for w := 1; w <= 4; w++ {
			go worker(w, jobs)
		}

		for s := 0; s < len(config.Sites); s++ {
			sites = append(sites, Site{URL: config.Sites[s], Status: 0})
		}

		perform(jobs)
		<-done

		return nil
	}

	app.Run(os.Args)
}

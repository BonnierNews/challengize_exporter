package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	userPoints = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "challengize_user_points",
		Help: "Number of points",
	}, []string{"user", "team", "stage"})
	lastRefresh = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "challengize_last_refresh",
		Help: "Timestamp of last successful refresh",
	})
)

func collectAll() error {
	var errs error
	for s := 1; s <= 4; s++ {
		err := collect(s)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

func collect(stage int) error {
	timestamp := time.Now().Unix()
	url := fmt.Sprintf("https://www.challengize.com/dashboard.action?getUserTableData&selectedStage=%d&_=%d", stage, timestamp)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	sessionCookie := &http.Cookie{
		Name:  "JSESSIONID",
		Value: os.Getenv("JSESSIONID"),
	}
	req.AddCookie(sessionCookie)
	rememberCookie := &http.Cookie{
		Name:  "remember",
		Value: os.Getenv("REMEMBER"),
	}
	req.AddCookie(rememberCookie)
	req.Header.Add("Accept", "application/json;charset=UTF-8")

	var client = &http.Client{
		// Don't follow redirects
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)

	if err != nil {
		return err
	} else if resp.StatusCode != 200 {
		return errors.New(fmt.Sprintf("Non-OK status code: %d\n", resp.StatusCode))
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	challengizeResponse := struct {
		Data []struct {
			User struct {
				PercentageAndPoints struct {
					Points int `json:"points"`
				} `json:"percentageAndPoints"`
				IdNameAvatar struct {
					Name     string `json:"name"`
					TeamName string `json:"teamName"`
				} `json:"idNameAvatar"`
			} `json:"user"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(data, &challengizeResponse)
	if err != nil {
		return err
	}

	for _, user := range challengizeResponse.Data {
		username := user.User.IdNameAvatar.Name
		team := user.User.IdNameAvatar.TeamName
		points := user.User.PercentageAndPoints.Points
		userPoints.WithLabelValues(username, team, strconv.Itoa(stage)).Set(float64(points))
	}

	return nil
}

func scheduleCollection() {
	ticker := time.NewTicker(15 * time.Minute)
	for {
		log.Println("Refreshing points")
		err := collectAll()
		if err == nil {
			lastRefresh.Set(float64(time.Now().Unix()))
		} else {
			log.Println(err)
		}

		select {
		case <-ticker.C:
			// Wait for next refresh
		}
	}
}

func main() {
	prometheus.MustRegister(userPoints)
	go scheduleCollection()

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":8080", nil)
}

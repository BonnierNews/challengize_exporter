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
	teamPoints = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "challengize_team_points",
		Help: "Number of points for team",
	}, []string{"team", "stage"})
	lastRefresh = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "challengize_last_refresh",
		Help: "Timestamp of last successful refresh",
	})
)

func collectAll() error {
	var errs error
	for s := 0; s <= 2; s++ { // TODO: Determine max available stage (team 404s for future stages)
		err := collectUser(s)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
		err = collectTeam(s)
		if err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

func getData(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
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
		return nil, err
	} else if resp.StatusCode != 200 {
		return nil, errors.New(fmt.Sprintf("Non-OK status code: %d\n", resp.StatusCode))
	}

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func collectUser(stage int) error {
	timestamp := time.Now().Unix()
	url := fmt.Sprintf("https://www.challengize.com/dashboard.action?getUserTableData&selectedStage=%d&_=%d", stage, timestamp)
	data, err := getData(url)
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

func collectTeam(stage int) error {
	timestamp := time.Now().Unix()
	url := fmt.Sprintf("https://www.challengize.com/dashboard.action?getTeamTableData&selectedStage=%d&_=%d", stage, timestamp)
	data, err := getData(url)
	if err != nil {
		return err
	}

	challengizeResponse := struct {
		Data []struct {
			Team struct {
				PercentageAndPoints struct {
					Points int `json:"points"`
				} `json:"percentageAndPoints"`
				NameAndId struct {
					TeamName string `json:"teamName"`
				} `json:"nameAndId"`
			} `json:"team"`
		} `json:"data"`
	}{}
	err = json.Unmarshal(data, &challengizeResponse)
	if err != nil {
		return err
	}

	for _, team := range challengizeResponse.Data {
		teamname := team.Team.NameAndId.TeamName
		points := team.Team.PercentageAndPoints.Points
		teamPoints.WithLabelValues(teamname, strconv.Itoa(stage)).Set(float64(points))
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
	prometheus.MustRegister(teamPoints)
	prometheus.MustRegister(lastRefresh)

	go scheduleCollection()

	http.Handle("/metrics", promhttp.Handler())
	http.ListenAndServe(":8080", nil)
}

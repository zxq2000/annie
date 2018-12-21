package mgtv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/iawia002/annie/downloader"
	"github.com/iawia002/annie/request"
	"github.com/iawia002/annie/utils"
)

type mgtvVideoStream struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Def  string `json:"def"`
}

type mgtvVideoInfo struct {
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

type mgtvVideoData struct {
	Stream       []mgtvVideoStream `json:"stream"`
	StreamDomain []string          `json:"stream_domain"`
	Info         mgtvVideoInfo     `json:"info"`
}

type mgtv struct {
	Data mgtvVideoData `json:"data"`
}

type mgtvVideoAddr struct {
	Info string `json:"info"`
}

type mgtvURLInfo struct {
	URL  string
	Size int64
}

type mgtvPm2Data struct {
	Data struct {
		Atc struct {
			Pm2 string `json:"pm2"`
		} `json:"atc"`
		Info mgtvVideoInfo `json:"info"`
	} `json:"data"`
}

func mgtvM3u8(url string) ([]mgtvURLInfo, int64, error) {
	var data []mgtvURLInfo
	var temp mgtvURLInfo
	var size, totalSize int64
	urls, err := utils.M3u8URLs(url)
	if err != nil {
		return nil, 0, err
	}
	m3u8String, err := request.Get(url, url, nil)
	if err != nil {
		return nil, 0, err
	}
	sizes := utils.MatchAll(m3u8String, `#EXT-MGTV-File-SIZE:(\d+)`)
	// sizes: [[#EXT-MGTV-File-SIZE:1893724, 1893724]]
	for index, u := range urls {
		size, err = strconv.ParseInt(sizes[index][1], 10, 64)
		if err != nil {
			return nil, 0, err
		}
		totalSize += size
		temp = mgtvURLInfo{
			URL:  u,
			Size: size,
		}
		data = append(data, temp)
	}
	return data, totalSize, nil
}

func encodeTk2(str string) string {
	encodeString := base64.StdEncoding.EncodeToString([]byte(str))
	r1 := regexp.MustCompile(`/\+/g`)
	r2 := regexp.MustCompile(`/\//g`)
	r3 := regexp.MustCompile(`/=/g`)
	r1.ReplaceAllString(encodeString, "_")
	r2.ReplaceAllString(encodeString, "~")
	r3.ReplaceAllString(encodeString, "-")
	encodeString = utils.Reverse(encodeString)
	return encodeString
}

// Extract is the main function for extracting data
func Extract(url string) ([]downloader.Data, error) {
	html, err := request.Get(url, url, nil)
	if err != nil {
		return downloader.EmptyList, err
	}
	vid := utils.MatchOneOf(
		url,
		`https?://www.mgtv.com/(?:b|l)/\d+/(\d+).html`,
		`https?://www.mgtv.com/hz/bdpz/\d+/(\d+).html`,
	)
	if vid == nil {
		vid = utils.MatchOneOf(html, `vid: (\d+),`)
	}
	// API extract from https://js.mgtv.com/imgotv-miniv6/global/page/play-tv.js
	// getSource and getPlayInfo function
	// Chrome Network JS panel
	headers := map[string]string{
		"Cookie": "PM_CHKID=1",
	}
	clit := fmt.Sprintf("clit=%d", time.Now().Unix()/1000)
	pm2DataString, err := request.Get(
		fmt.Sprintf(
			"https://pcweb.api.mgtv.com/player/video?video_id=%s&tk2=%s",
			vid[1],
			encodeTk2(fmt.Sprintf(
				"did=f11dee65-4e0d-4d25-bfce-719ad9dc991d|pno=1030|ver=5.5.1|%s", clit,
			)),
		),
		url,
		headers,
	)
	if err != nil {
		return downloader.EmptyList, err
	}
	var pm2 mgtvPm2Data
	json.Unmarshal([]byte(pm2DataString), &pm2)
	dataString, err := request.Get(
		fmt.Sprintf(
			"https://pcweb.api.mgtv.com/player/getSource?video_id=%s&tk2=%s&pm2=%s",
			vid[1], encodeTk2(clit), pm2.Data.Atc.Pm2,
		),
		url,
		headers,
	)
	if err != nil {
		return downloader.EmptyList, err
	}
	var mgtvData mgtv
	json.Unmarshal([]byte(dataString), &mgtvData)
	title := strings.TrimSpace(
		pm2.Data.Info.Title + " " + pm2.Data.Info.Desc,
	)
	mgtvStreams := mgtvData.Data.Stream
	var addr mgtvVideoAddr
	streams := map[string]downloader.Stream{}
	for _, stream := range mgtvStreams {
		if stream.URL == "" {
			continue
		}
		// real download address
		addr = mgtvVideoAddr{}
		addrInfo, err := request.Get(mgtvData.Data.StreamDomain[0]+stream.URL, url, headers)
		if err != nil {
			return downloader.EmptyList, err
		}
		json.Unmarshal([]byte(addrInfo), &addr)
		m3u8URLs, totalSize, err := mgtvM3u8(addr.Info)
		if err != nil {
			return downloader.EmptyList, err
		}
		urls := make([]downloader.URL, len(m3u8URLs))
		for index, u := range m3u8URLs {
			urls[index] = downloader.URL{
				URL:  u.URL,
				Size: u.Size,
				Ext:  "ts",
			}
		}
		streams[stream.Def] = downloader.Stream{
			URLs:    urls,
			Size:    totalSize,
			Quality: stream.Name,
		}
	}

	return []downloader.Data{
		{
			Site:    "芒果TV mgtv.com",
			Title:   title,
			Type:    "video",
			Streams: streams,
			URL:     url,
		},
	}, nil
}
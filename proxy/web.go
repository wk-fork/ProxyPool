package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"time"

	gg "github.com/haozibi/gglog"
)

var (
	port          = "9090"
	redisZaddName = "healthEvalue"
	timeLayout    = "2006-01-02 15:04:05"
)

// 以 web 的形式启动代理筛选
func StartProxyByWeb() {
	go startProxy(true)
	dialRedis()
	defer connRedis.Close()

	http.HandleFunc("/", index)
	http.HandleFunc("/get", getIP)
	http.HandleFunc("/delete", deleteIP) // /delete?ip=123.123.123.123:1024

	gg.Infoln("listen...")
	err := http.ListenAndServe(":"+port, nil)
	if err != nil {
		panic(err)
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		fmt.Fprintf(w, "404 page not found\n")
		return
	}
	fmt.Fprintf(w, "hello world\n")
	return
}

func getIP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("content-type", "application/json")
	s := getWebList()
	// w.Write()
	fmt.Fprintf(w, s)
	return
}

// /delete?ip=123.123.123.123:1024
func deleteIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		fmt.Fprintf(w, "404 page not found\n")
		return
	}
	r.ParseForm()
	ip := r.Form.Get("ip")
	if m, _ := regexp.MatchString(regexIPPort, ip); !m {
		fmt.Fprintf(w, "Url %v not match regex\n", ip)
		return
	}
	if deleteWebList(ip) {
		fmt.Fprintf(w, "ok")
		return
	}
	fmt.Fprintf(w, "%v,error", ip)
	return
}

// 把 IP 添加到 Redis 中
func addWebList(uri string) {
	if m, _ := regexp.MatchString(regexIPPort, uri); !m {
		gg.Errorf("Url %v not match regex\n", uri)
		return
	}
	var r proxyJson

	s, err := getRString(uri)
	if err == nil {
		// 说明存在，更新数据
		json.Unmarshal([]byte(s), &r)
		okCount := float64(r.TestCount) * float64(r.HealthEvaluation) * 0.01 // float64
		r.TestCount = r.TestCount + 1
		r.HealthEvaluation = int(((okCount + 1) / float64(r.TestCount)) * 100)
		r.UpdatedAt = time.Now().Format(timeLayout)
	} else {
		r = proxyJson{
			IP:               uri,
			Class:            0,
			ClassName:        "http",
			HealthEvaluation: 100,
			TestCount:        1,
			CreadtedAt:       time.Now().Format(timeLayout),
			UpdatedAt:        time.Now().Format(timeLayout),
		}
	}

	tmpS, err := json.Marshal(r)
	if err != nil {
		gg.Errorf("Marshal %v error,%v\n", uri, err)
		return
	}
	err = setRString(uri, string(tmpS))
	if err != nil {
		gg.Errorf("Add string %v error,%v\n", uri, err)
		return
	}
	// 更新添加值
	err = setRSortSet(r.HealthEvaluation, uri)
	if err != nil {
		gg.Errorf("Add set %v error,%v\n", uri, err)
		return
	}
	return
}

// 必定返回一个正确的数据
// 如果发生错误则重新获取，尝试5次
func getWebList() string {
	if getRSortSetNum() == 0 {
		return `{error:"not found available ip"}`
	}
	var i = 0
	for i < 5 {
		m, err := getRSortSet(i, i)
		i = i + 1
		if err != nil {
			gg.Errorf("Get sort set error,%v\n", err)
			continue
		}
		a, err := getRString(m[0])
		if err != nil {
			gg.Errorf("Get redis value error,%v\n", err)
			if deleteWebList(m[0]) {
				gg.Debugf("Get redis value error,so delte %v\n", m[0])
			}
			continue
		}
		return a
	}
	return ""
}

func deleteWebList(uri string) bool {
	if m, _ := regexp.MatchString(regexIPPort, uri); !m {
		gg.Errorf("Url %v not match regex\n", uri)
		return false
	}
	if deleteRString(uri) == 0 || deleteRSortSet(uri) == 0 {
		gg.Errorf("Delete %v error", uri)
		return false
	}
	return true
}

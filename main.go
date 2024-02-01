package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sync"
	"time"

	"gopkg.in/gomail.v2"
)

var (
	config      Config
	configMutex sync.RWMutex
)

const configFile = "config.json" // 配置文件路径

// Config 结构用于存储配置信息
type Config struct {
	ListenAddr  string        `json:"listenAddr"`
	Recipients  []string      `json:"recipients"`
	IntervalStr string        `json:"interval"`
	TimeoutStr  string        `json:"timeout"`
	EmailFrom   string        `json:"emailFrom"`
	EmailPass   string        `json:"emailPass"`
	Interval    time.Duration `json:"-"`
	Timeout     time.Duration `json:"-"`
}

func (c *Config) parseDuration() error {
	var err error
	c.Interval, err = time.ParseDuration(c.IntervalStr)
	if err != nil {
		return err
	}

	c.Timeout, err = time.ParseDuration(c.TimeoutStr)
	return err
}

func loadConfig() error {
	configMutex.Lock()
	defer configMutex.Unlock()

	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	// 手动解析 duration 字段
	err = config.parseDuration()
	if err != nil {
		return err
	}

	return nil
}

var (
	lastCall    = time.Now() // 记录最后一次调用的时间
	timeoutFlag = false      // 标志是否已经发送了超时邮件
	mutex       sync.Mutex   // 用于保护 timeoutFlag 的互斥锁
)

type APIResponse struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func sendEmail(subject, body string) {
	m := gomail.NewMessage()
	m.SetHeader("From", config.EmailFrom) // 发件人邮箱
	m.SetHeader("To", config.Recipients...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer("smtp.qq.com", 465, config.EmailFrom, config.EmailPass) // 替换成你的SMTP服务器信息

	if err := d.DialAndSend(m); err != nil {
		fmt.Println("Error sending email:", err)
	}
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
	// 解析参数
	param1 := r.FormValue("param1")
	param2 := r.FormValue("param2")

	// 更新最后一次调用的时间
	lastCall = time.Now()

	// 进行一些处理，这里只是简单地打印参数,记上时间
	fmt.Printf("%s Received parameters: param1=%s, param2=%s\n", time.Now().Format("2006-01-02 15:04:05"), param1, param2)

	// 如果之前已经发送过邮件，则设置标志并发送恢复邮件
	mutex.Lock()
	if timeoutFlag {
		sendEmail("API Recovered", "API has been called by zabbix and recovered.")
		timeoutFlag = false
	}
	mutex.Unlock()

	// 构建并返回 API 响应
	//apiResponse := APIResponse{
	//	Message: "API called successfully",
	//	Code:    233,
	//}

	apiResponse := 233

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(apiResponse)
}

func checkTimeout() {
	for {
		time.Sleep(config.Interval)
		//fmt.Printf(time.Now().Format("2006-01-02 15:04:05"))
		if time.Since(lastCall) > config.Timeout {
			// 如果之前没有发送过邮件，则设置标志并发送超时邮件
			mutex.Lock()
			if !timeoutFlag {
				sendEmail("API Timeout", "API has not been called by zabbix in the last 10 minutes.")
				timeoutFlag = true
			}
			mutex.Unlock()
		}
	}
}

func main() {
	// 从配置文件加载配置
	if err := loadConfig(); err != nil {
		fmt.Println("Error loading config:", err)
		return
	}
	// 启动定时检查超时的 goroutine
	go checkTimeout()

	// 注册 API 处理函数
	http.HandleFunc("/call-api", handleRequest)

	// 启动 HTTP 服务器
	err := http.ListenAndServe(config.ListenAddr, nil)
	if err != nil {
		fmt.Println("Error starting HTTP server:", err)
	}
}

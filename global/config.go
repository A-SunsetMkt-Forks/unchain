package global

import (
	"bytes"
	"fmt"
	"github.com/BurntSushi/toml"
	"log"
	"log/slog"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	SubAddresses      string `desc:"sub addresses" def:"112.115.112.2:80,node2.xxx.cn:443"`                                            //这个信息会帮助你生成V2ray/Clash/ShadowRocket的订阅链接,同时这个是互联网浏览器访问的地址
	Port              string `desc:"port" def:"80"`                                                                                    //golang app 服务端口,可选,建议默认80或者443
	RegisterUrl       string `desc:"register url" def:"https://unchainapi.bob99.workers.dev/api/node"`                                 //optional,流量,用户鉴权的主控服务器地址
	RegisterToken     string `desc:"register token" def:"unchain people from censorship and surveillance"`                             //optional,流量,用户鉴权的主控服务器token
	AllowUsers        string `desc:"allow users UUID" def:"903bcd04-79e7-429c-bf0c-0456c7de9cdc,903bcd04-79e7-429c-bf0c-0456c7de9cd1"` //单机模式下,允许的用户UUID
	LogFile           string `desc:"log file path" def:""`                                                                             //日志文件路径
	DebugLevel        string `desc:"debug level" def:"DEBUG"`                                                                          //日志级别
	IntervalSecond    string `desc:"interval second" def:"3600"`                                                                       //seconds 向主控服务器推送,流量使用情况的间隔时间
	GitHash           string `desc:"git hash" def:""`                                                                                  //optional git hash
	BuildTime         string `desc:"build time" def:""`                                                                                //optional build time
	RunAt             string `desc:"run at" def:""`                                                                                    //optional run at
	IsUserTrafficStat string `desc:"is user traffic stat" def:"false"`                                                                 //是否开启用户流量统计,使用true 开启用户流量统计,使用false 关闭用户流量统计
}

func (c Config) DisableUserTraffic() bool {
	if strings.ToLower(c.IsUserTrafficStat) == "true" {
		return true
	}
	return false
}

func (c Config) SubHostWithPort() []string {
	parts := strings.Split(c.SubAddresses, ",")
	ids := make([]string, 0)
	for _, addr := range parts {
		addr = strings.TrimSpace(addr)
		addr = strings.TrimPrefix(addr, "https://")
		addr = strings.TrimPrefix(addr, "http://")
		if addr != "" {
			if !strings.Contains(addr, ":") {
				addr = fmt.Sprintf("%s:%d", addr, 443)
			}
			ids = append(ids, addr)
		}
	}
	return ids
}
func (c Config) ListenAddr() string {
	return fmt.Sprintf("0.0.0.0:%s", c.Port)
}
func (c Config) PushIntervalSecond() int {
	iv, err := strconv.ParseInt(c.IntervalSecond, 10, 32)
	if err != nil {
		log.Println("failed to parse interval second:", err)
		return 3600
	}
	return int(iv)
}

func loadEnv() *Config {
	opt := Config{}
	for i := 0; i < reflect.TypeOf(opt).NumField(); i++ {
		propertyName := reflect.TypeOf(opt).Field(i).Name
		key := snakeCaseUpper(propertyName)
		def := reflect.TypeOf(opt).Field(i).Tag.Get("def")
		desc := reflect.TypeOf(opt).Field(i).Tag.Get("desc")
		vv := osEnvWithDefault(key, desc, def)
		reflect.ValueOf(&opt).Elem().Field(i).SetString(vv)
	}
	return &opt
}

func snakeCase(camel string) string {
	var buf bytes.Buffer
	for _, c := range camel {
		if 'A' <= c && c <= 'Z' {
			// just convert [A-Z] to _[a-z]
			if buf.Len() > 0 {
				buf.WriteRune('_')
			}
			buf.WriteRune(c - 'A' + 'a')
		} else {
			buf.WriteRune(c)
		}
	}
	return buf.String()
}

func snakeCaseUpper(camel string) string {
	return strings.ToUpper(snakeCase(camel))
}

func osEnvWithDefault(key, desc, def string) string {
	if v := os.Getenv(key); v == "" {
		fmt.Printf("%s <%s> 默认:  %s\n", key, desc, def)
		return def
	} else {
		return v
	}
}

func (c Config) ListenPort() int {
	iv, err := strconv.ParseInt(c.Port, 10, 32)
	if err != nil {
		log.Println("failed to parse port:", err)
		return 80
	}
	return int(iv)
}

var (
	gitHash   string
	buildTime string
)

var cfg *Config

// Cfg load config from toml file or env
func Cfg(tomlFilePath string) *Config {
	if cfg != nil {
		return cfg
	}
	cfgIns, err := loadFromToml(tomlFilePath)
	if err != nil {
		fmt.Println(tomlFilePath, err)
		fmt.Println("unable to load config file form config.toml file, use env instead")
		cfg = loadEnv()
	} else {
		cfg = cfgIns
	}
	cfg.GitHash = gitHash
	cfg.BuildTime = buildTime
	cfg.RunAt = time.Now().Format("2006-01-02 15:04:05")
	return cfg
}

func loadFromToml(file string) (*Config, error) {
	opt := Config{}
	_, err := toml.DecodeFile(file, &opt)
	if err != nil {
		return nil, fmt.Errorf("failed to load config file:%s %w", file, err)
	}
	return &opt, nil
}

func (c Config) LogLevel() slog.Level {
	l := slog.LevelDebug
	switch strings.ToUpper(c.DebugLevel) {
	case "DEBUG":
		l = slog.LevelDebug
	case "INFO":
		l = slog.LevelInfo
	case "WARN":
		l = slog.LevelWarn
	case "ERROR":
		l = slog.LevelError
	default:
		l = slog.LevelError
	}
	return l
}
func (c Config) UserIDS() []string {
	parts := strings.Split(c.AllowUsers, ",")
	ids := make([]string, 0)
	for _, uid := range parts {
		uid = strings.TrimSpace(uid)
		if uid != "" {
			ids = append(ids, uid)
		}
	}
	return ids
}

func (c Config) PushInterval() time.Duration {
	if c.PushIntervalSecond() <= 0 {
		return time.Minute * 60
	}
	return time.Second * time.Duration(c.PushIntervalSecond())
}

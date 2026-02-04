package main

import (
	"bufio"
	_ "embed"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/getlantern/systray"
	"github.com/go-vgo/robotgo"
	"github.com/gofrs/flock"
	hook "github.com/robotn/gohook"
)

//go:embed icon.ico
var trayIcon []byte

// 粘贴快捷键
var pasteKeys []string

// 文件锁，防止多实例运行
var fileLock = flock.New("app.lock")

// 热键列表
var hotkeys = make(map[string]string)

func main() {
	getlock()
	initPasteKeys()
	readConfig()
	go listen(hotkeys)
	systray.Run(onReady, onExit)
}

func getlock() {
	locked, err := fileLock.TryLock()
	if err != nil {
		log.Fatalf("无法获取文件锁: %s", err)
	}
	if !locked {
		log.Fatalf("无法获取文件锁，可能是因为应用程序已在运行")
	}
}

func initPasteKeys() {
	switch runtime.GOOS {
	case "darwin": // macOS
		pasteKeys = []string{"cmd", "v"}
	case "windows", "linux":
		pasteKeys = []string{"ctrl", "v"}
	default:
		pasteKeys = []string{"ctrl", "v"} // 默认 fallback
	}
}

func readConfig() {
	// 读取 config.txt 文件
	configPath := "config.txt"
	file, err := os.Open(configPath)
	if err != nil {
		// 读取文件失败，创建本地示例文件
		sampleContent := "# 每行对应一条热键映射规则，格式为：热键序列=要粘贴的文本\n# 例如：hw=你好，世界！ 你可以试着在任意地方输入hw看看效果\n# 注意：热键序列应尽可能只包含小写字母和数字，以免引发意外错误\n# 井号开头的行是注释行，会被忽略。\n\nhw=你好，世界！"
		err = os.WriteFile(configPath, []byte(sampleContent), 0644)
		if err != nil {
			log.Fatalf("无法创建示例配置文件: %s", err)
		}
		readConfig() // 重新读取配置文件
	}
	defer file.Close()

	fileScanner := bufio.NewScanner(file)

	// read line by line
	for fileScanner.Scan() {
		lineText := fileScanner.Text()

		// 如果是空行或者#开头的注释行，则跳过
		if !(len(lineText) == 0 || strings.HasPrefix(lineText, "#")) {
			i := strings.Index(lineText, "=")
			if i > 0 {
				key := strings.TrimSpace(lineText[:i])
				value := strings.TrimSpace(lineText[i+1:])
				hotkeys[key] = value
			}
		}
	}

	// handle first encountered error while reading
	if err := fileScanner.Err(); err != nil {
		log.Fatalf("Error while reading file: %s", err)
	}
}

func onReady() {
	systray.SetTemplateIcon(trayIcon, trayIcon)
	systray.SetTitle("hotword 热词助手")
	systray.SetTooltip("hotword 热词助手")

	mOpen := systray.AddMenuItem("打开目录", "打开目录")
	mReload := systray.AddMenuItem("重载配置", "重载配置")
	mQuit := systray.AddMenuItem("退出应用", "退出应用")

	// 监听点击事件
	go func() {
		for {
			select {
			case <-mOpen.ClickedCh:
				openDir()
			case <-mReload.ClickedCh:
				// 取消现有注册的热键、清空热键列表，重新读取配置，并监听新的热键
				hook.End()
				hotkeys = make(map[string]string)
				readConfig()
				go listen(hotkeys)
			case <-mQuit.ClickedCh:
				systray.Quit()
			}
		}
	}()
}

func onExit() {
	hook.End()
	fileLock.Unlock()
}

func openDir() {
	var cmd string
	var args []string

	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("无法获取当前目录: %s", err)
	}

	switch runtime.GOOS {
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	case "darwin":
		cmd = "open"
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = "xdg-open"
	}

	args = append(args, dir)

	c := exec.Command(cmd, args...)
	c.Start()
}

// 闭包函数，监听指定按键序列
func listenKeys(keyList []rune, pasteText string) func(key rune) {
	var keySerial = []rune{}

	return func(key rune) {
		if key == keyList[len(keySerial)] {
			keySerial = append(keySerial, key)

			if len(keySerial) == len(keyList) {
				// 1. 删除已输入的按键序列
				for range len(keyList) {
					robotgo.KeyTap("backspace")
				}
				// 2. 存入剪贴板内容
				robotgo.WriteAll(pasteText)
				// 3. 执行粘贴操作
				robotgo.KeyTap(pasteKeys[1], pasteKeys[0])
				// 4. 重置输入序列
				keySerial = []rune{}
			}
		} else if len(keySerial) == 1 && key == keyList[0] {
			return // 忽略重复的第一个按键
		} else {
			keySerial = []rune{} // 重置输入序列
		}
	}
}

func listen(hotkeys map[string]string) {
	// 注册并监听热键序列
	for keySeq, pasteText := range hotkeys {
		keyList := []rune(keySeq)
		fun := listenKeys(keyList, pasteText)

		hook.Register(hook.KeyUp, []string{}, func(e hook.Event) {
			s := hook.RawcodetoKeychar(e.Rawcode)
			if len(s) == 1 {
				fun(rune(s[0]))
			} else {
				return
			}

		})
	}

	s := hook.Start()
	<-hook.Process(s)
}

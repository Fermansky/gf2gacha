package util

import (
	"compress/gzip"
	"fmt"
	"gf2gacha/model"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"
)

// GetBinLogInfo 我们可能从binlog中获取到 抽卡记录链接 和 用户UID，AccessToken暂不知抓包外的获取手段
func GetBinLogInfo() (logInfo model.LogInfo, err error) {
	gameDataDir, err := GetGameDataDir()
	if err != nil {
		return model.LogInfo{}, err
	}
	parentDir := filepath.Dir(gameDataDir)
	logDir := filepath.Join(parentDir, "Log")
	lastBin, err := findLatestLastBin(logDir)
	if err != nil {
		return model.LogInfo{}, err
	}

	dst := "binlog_unpacked.tmp" // 解压后的原始文件

	// 解压 GZIP 文件
	err = decompressGzip(lastBin, dst)
	if err != nil {
		return model.LogInfo{}, err
	}

	// 读取并解析 UTF-16LE
	content, err := os.ReadFile(dst)
	if err != nil {
		return model.LogInfo{}, err
	}

	text := tryUTF16LE(content)

	gachaUrl, err := extractGachaUrl(text)
	logInfo.GachaUrl = gachaUrl

	uid, err := extractGF2UID(text)
	logInfo.Uid = uid

	os.Remove(dst)

	return logInfo, nil
}

func findLatestLastBin(dir string) (string, error) {
	var latestPath string
	var latestModTime time.Time

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if strings.HasSuffix(info.Name(), "last.bin") {
			if latestPath == "" || info.ModTime().After(latestModTime) {
				latestPath = path
				latestModTime = info.ModTime()
			}
		}
		return nil
	})

	if err != nil {
		return "", err
	}

	if latestPath == "" {
		return "", fmt.Errorf("未找到任何 last.bin 文件")
	}

	return latestPath, nil
}

func extractGachaUrl(text string) (string, error) {
	re, err := regexp.Compile(`"gacha_record_url":"(.*?)"`)
	if err != nil {
		return "", err
	}

	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("未在BinLog文件中找到 gacha_record_url")
	}

	last := matches[len(matches)-1]
	if len(last) > 1 {
		return last[1], nil
	}
	return "", fmt.Errorf("未在BinLog文件中找到 gacha_record_url")
}

func extractGF2UID(text string) (string, error) {
	re, err := regexp.Compile(`Key:\s*GF2UID\s*,\s*Value:\s*(\d+)`)
	if err != nil {
		return "", err
	}

	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("未找到 GF2UID")
	}

	last := matches[len(matches)-1]
	if len(last) > 1 {
		return last[1], nil
	}

	return "", fmt.Errorf("未找到 GF2UID")
}

func decompressGzip(src string, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	gzr, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer gzr.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, gzr)
	return err
}

func tryUTF16LE(b []byte) string {
	if len(b)%2 != 0 {
		b = b[:len(b)-1]
	}
	u16 := make([]uint16, len(b)/2)
	for i := 0; i < len(u16); i++ {
		u16[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u16))
}

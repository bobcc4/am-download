package lyrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/beevik/etree"
)

type SongLyrics struct {
	Data []struct {
		Id         string `json:"id"`
		Type       string `json:"type"`
		Attributes struct {
			Ttml              string `json:"ttml"`
			TtmlLocalizations string `json:"ttmlLocalizations"`
			PlayParams        struct {
				Id          string `json:"id"`
				Kind        string `json:"kind"`
				CatalogId   string `json:"catalogId"`
				DisplayType int    `json:"displayType"`
			} `json:"playParams"`
		} `json:"attributes"`
	} `json:"data"`
}

// Get 获取歌词入口
func normalizeLyricMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "word", "syllable", "逐字":
		return "word"
	case "line", "逐行":
		return "line"
	default:
		return ""
	}
}

func Get(storefront, songId, lrcType, language, lrcFormat, token, mediaUserToken string, conversionMode ...string) (string, error) {
	if len(mediaUserToken) < 50 {
		return "", errors.New("MediaUserToken not set")
	}

	// === 策略修改 ===
	// 根据您的反馈，script 参数设置为 "en-Latn" 以正确触发罗马音返回
	mode := ""
	if len(conversionMode) > 0 {
		mode = normalizeLyricMode(conversionMode[0])
	}
	script := "en-Latn"
	endpoint := "syllable-lyrics"
	fallbackEndpoint := "lyrics"
	fallbackScript := ""
	if mode == "line" {
		script = ""
		endpoint = "lyrics"
		fallbackEndpoint = "syllable-lyrics"
		fallbackScript = "en-Latn"
	} else if mode == "" && (lrcType == "lyrics" || lrcType == "syllable-lyrics") {
		endpoint = lrcType
		if endpoint == "lyrics" {
			fallbackEndpoint = "syllable-lyrics"
			fallbackScript = "en-Latn"
		}
	}

	// 1. 优先尝试获取 syllable-lyrics (带 en-Latn 参数)
	ttml, err := fetchLyricsInternal(songId, storefront, token, mediaUserToken, endpoint, language, script)

	// 2. 降级策略：如果失败 (err != nil) 或者返回内容为空，则降级尝试标准 lyrics 接口 (不带 script)
	// 这可以防止不支持逐字歌词的歌曲直接下载失败
	if err != nil || ttml == "" {
		ttml, err = fetchLyricsInternal(songId, storefront, token, mediaUserToken, fallbackEndpoint, language, fallbackScript)
		if err != nil {
			return "", err // 如果标准歌词也失败，则彻底失败
		}
	}

	if lrcFormat == "ttml" {
		return ttml, nil
	}

	// 解析 TTML 为 LRC
	lrc, err := TtmlToLrc(ttml, mode)
	if err != nil {
		return "", err
	}

	return lrc, nil
}

// fetchLyricsInternal 封装底层的 HTTP 请求逻辑
func fetchLyricsInternal(songId, storefront, token, userToken, endpointType, language, script string) (string, error) {
	// 构建 URL
	// 格式参考：https://amp-api.music.apple.com/v1/catalog/{storefront}/songs/{songId}/{endpoint}?l={language}&extend=ttmlLocalizations&l[script]={script}
	baseUrl := fmt.Sprintf("https://amp-api.music.apple.com/v1/catalog/%s/songs/%s/%s?l=%s&extend=ttmlLocalizations",
		storefront, songId, endpointType, language)

	// 仅当 script 不为空时添加 l[script] 参数
	if script != "" {
		baseUrl += fmt.Sprintf("&l[script]=%s", script)
	}

	req, err := http.NewRequest("GET", baseUrl, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Origin", "https://music.apple.com")
	req.Header.Set("Referer", "https://music.apple.com/")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	cookie := http.Cookie{Name: "media-user-token", Value: userToken}
	req.AddCookie(&cookie)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 返回错误以便上层捕获并进行降级重试
		return "", fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	obj := new(SongLyrics)
	if err := json.NewDecoder(resp.Body).Decode(&obj); err != nil {
		return "", err
	}

	if obj.Data != nil && len(obj.Data) > 0 {
		attrs := obj.Data[0].Attributes
		if len(attrs.Ttml) > 0 {
			return attrs.Ttml, nil
		}
		if len(attrs.TtmlLocalizations) > 0 {
			return attrs.TtmlLocalizations, nil
		}
	}

	return "", errors.New("no lyrics data found in response")
}

// ================= 辅助工具函数 =================

func shouldKeepText(data string) bool {
	if len(data) == 0 {
		return false
	}
	if strings.TrimSpace(data) != "" {
		return true
	}
	isStructureFormatting := true
	for _, char := range data {
		if char != '\n' && char != '\r' && char != '\t' {
			isStructureFormatting = false
			break
		}
	}
	return !isStructureFormatting
}

func getAllText(element *etree.Element) string {
	var sb strings.Builder
	for _, child := range element.Child {
		switch c := child.(type) {
		case *etree.CharData:
			sb.WriteString(c.Data)
		case *etree.Element:
			sb.WriteString(getAllText(c))
		}
	}
	return sb.String()
}

func getSplitText(element *etree.Element) (string, string) {
	var mainSb, bgSb strings.Builder
	for _, child := range element.Child {
		switch c := child.(type) {
		case *etree.CharData:
			mainSb.WriteString(c.Data)
		case *etree.Element:
			role := c.SelectAttrValue("ttm:role", "")
			if role == "x-bg" {
				bgSb.WriteString(getAllText(c))
			} else {
				m, b := getSplitText(c)
				mainSb.WriteString(m)
				bgSb.WriteString(b)
			}
		}
	}
	return mainSb.String(), bgSb.String()
}

func parseLrcTime(value string) (string, error) {
	var h, m, s, ms int
	var err error
	if strings.Contains(value, ":") {
		_, err = fmt.Sscanf(value, "%d:%d:%d.%d", &h, &m, &s, &ms)
		if err != nil {
			_, err = fmt.Sscanf(value, "%d:%d.%d", &m, &s, &ms)
			h = 0
		}
	} else {
		_, err = fmt.Sscanf(value, "%d.%d", &s, &ms)
		h = 0
		m = 0
	}
	if err != nil {
		return "", err
	}
	m += h * 60
	ms = ms / 10
	return fmt.Sprintf("%02d:%02d.%02d", m, s, ms), nil
}

func makeSyllableStamp(t string) (string, error) {
	ts, err := parseLrcTime(t)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("<%s>", ts), nil
}

// extractTranslations 提取常规翻译
func extractTranslations(root *etree.Element) map[string]*etree.Element {
	translations := map[string]*etree.Element{}
	if root == nil {
		return translations
	}
	head := root.FindElement("head")
	if head == nil {
		return translations
	}
	meta := head.FindElement("metadata")
	if meta != nil {
		im := meta.FindElement("iTunesMetadata")
		if im != nil {
			trs := im.FindElement("translations")
			if trs != nil {
				for _, tr := range trs.FindElements("translation") {
					for _, txt := range tr.FindElements("text") {
						key := txt.SelectAttrValue("for", "")
						if key != "" {
							translations[key] = txt
						}
					}
				}
			}
		}
	}
	return translations
}

// extractTransliterations 提取罗马音 (ja-Latn)
func extractTransliterations(root *etree.Element) map[string]*etree.Element {
	transliterations := map[string]*etree.Element{}
	if root == nil {
		return transliterations
	}
	head := root.FindElement("head")
	if head == nil {
		return transliterations
	}
	meta := head.FindElement("metadata")
	if meta != nil {
		im := meta.FindElement("iTunesMetadata")
		if im != nil {
			trs := im.FindElement("transliterations")
			if trs != nil {
				for _, tr := range trs.FindElements("transliteration") {
					// 这里的 xml:lang 通常是 ja-Latn，即便请求参数是 en-Latn
					if tr.SelectAttrValue("xml:lang", "") == "ja-Latn" {
						for _, txt := range tr.FindElements("text") {
							key := txt.SelectAttrValue("for", "")
							if key != "" {
								transliterations[key] = txt
							}
						}
					}
				}
			}
		}
	}
	return transliterations
}

// ================= 主逻辑 =================

func TtmlToLrc(ttml string, modes ...string) (string, error) {
	parsedTTML := etree.NewDocument()
	err := parsedTTML.ReadFromString(ttml)
	if err != nil {
		return "", err
	}

	mode := ""
	if len(modes) > 0 {
		mode = normalizeLyricMode(modes[0])
	}
	root := parsedTTML.FindElement("tt")
	if root == nil {
		return "", errors.New("invalid TTML")
	}
	timingAttr := root.SelectAttr("itunes:timing")

	if timingAttr != nil && timingAttr.Value == "None" {
		return convertNoneTimingToText(parsedTTML)
	}

	if timingAttr != nil && timingAttr.Value == "Word" {
		if mode == "line" {
			return conventWordTTMLToLineLRC(parsedTTML)
		}
		return conventSyllableTTMLToLRC(parsedTTML)
	}

	return conventLineTTMLToLRC(parsedTTML)
}

// 处理 itunes:timing="None" 的情况，直接输出纯文本
func convertNoneTimingToText(parsed *etree.Document) (string, error) {
	root := parsed.FindElement("tt")
	if root == nil {
		return "", errors.New("invalid TTML")
	}
	body := root.FindElement("body")
	if body == nil {
		return "", errors.New("invalid TTML")
	}

	var lines []string
	for _, div := range body.FindElements("div") {
		for _, p := range div.FindElements("p") {
			// 直接获取文本，忽略所有时间属性
			text := getAllText(p)
			if text != "" {
				lines = append(lines, text)
			}
		}
	}

	// 如果你希望它看起来像一个“无时间戳”的 LRC，可以直接返回文本行
	// 或者在每行前面加上占位符 [00:00.00] 以保持格式统一
	return strings.Join(lines, "\n"), nil
}

// Line 模式处理逻辑
func conventLineTTMLToLRC(parsed *etree.Document) (string, error) {
	root := parsed.FindElement("tt")
	if root == nil {
		return "", errors.New("invalid TTML")
	}
	body := root.FindElement("body")
	if body == nil {
		return "", errors.New("invalid TTML")
	}

	var lrcLines []string
	isJa := root.SelectAttrValue("xml:lang", "") == "ja"
	isHant := root.SelectAttrValue("xml:lang", "") == "zh-Hant"

	translations := extractTranslations(root)
	transliterations := extractTransliterations(root)

	for _, div := range body.FindElements("div") {
		for _, p := range div.FindElements("p") {
			key := p.SelectAttrValue("itunes:key", "")
			pBegin := p.SelectAttrValue("begin", "")
			pEnd := p.SelectAttrValue("end", "")
			if pBegin == "" || pEnd == "" {
				continue
			}

			startTime, _ := parseLrcTime(pBegin)
			endTime, _ := parseLrcTime(pEnd)
			timeTag := fmt.Sprintf("[%s]", startTime)
			endTimeTag := fmt.Sprintf("[%s]", endTime)

			// 固定使用分离模式获取文本
			mainText, bgText := getSplitText(p)

			var mainTrans, bgTrans string
			if trElem, ok := translations[key]; ok && trElem != nil {
				mainTrans, bgTrans = getSplitText(trElem)
			}

			// [Patch] 修复合并的翻译 (括号提取 - 保留括号)
			// 仅当：存在背景文字 + 背景翻译为空 + 主唱翻译不为空
			if bgText != "" && bgTrans == "" && mainTrans != "" {
				if strings.HasSuffix(mainTrans, "）") {
					idx := strings.LastIndex(mainTrans, "（")
					if idx != -1 {
						bgTrans = strings.TrimSpace(mainTrans[idx:])
						mainTrans = strings.TrimSpace(mainTrans[:idx])
					}
				} else if strings.HasSuffix(mainTrans, ")") {
					idx := strings.LastIndex(mainTrans, "(")
					if idx != -1 {
						bgTrans = strings.TrimSpace(mainTrans[idx:])
						mainTrans = strings.TrimSpace(mainTrans[:idx])
					}
				}
			}

			// ================= 新增修改 (Line模式) =================
			mainTrans = strings.ReplaceAll(mainTrans, "（", "(")
			mainTrans = strings.ReplaceAll(mainTrans, "）", ")")
			bgTrans = strings.ReplaceAll(bgTrans, "（", "(")
			bgTrans = strings.ReplaceAll(bgTrans, "）", ")")
			// =====================================================
			// 获取罗马音 (仅当原文是日语且存在ja-Latn时)
			var mainRomaji, bgRomaji string
			if isJa {
				if romajiElem, ok := transliterations[key]; ok && romajiElem != nil {
					mainRomaji, bgRomaji = getSplitText(romajiElem)
				}
			}

			// 1. 原文行 (处理繁体中文特殊情况)
			if isHant && mainTrans != "" {
				lrcLines = append(lrcLines, fmt.Sprintf("%s%s%s", timeTag, mainTrans, endTimeTag))
				if bgTrans != "" {
					lrcLines = append(lrcLines, fmt.Sprintf("%s%s%s", timeTag, bgTrans, endTimeTag))
				}
				continue
			}

			if mainText != "" {
				lrcLines = append(lrcLines, fmt.Sprintf("%s%s%s", timeTag, mainText, endTimeTag))
			}
			if bgText != "" {
				lrcLines = append(lrcLines, fmt.Sprintf("%s%s%s", timeTag, bgText, endTimeTag))
			}

			// 2. 罗马音行 (插入在原文和翻译之间)
			if isJa && mainRomaji != "" {
				lrcLines = append(lrcLines, fmt.Sprintf("%s%s%s", timeTag, mainRomaji, endTimeTag))
				if bgRomaji != "" {
					lrcLines = append(lrcLines, fmt.Sprintf("%s%s%s", timeTag, bgRomaji, endTimeTag))
				}
			}

			// 3. 翻译行
			if !(isHant && mainTrans != "") {
				if mainTrans != "" {
					lrcLines = append(lrcLines, fmt.Sprintf("%s%s%s", timeTag, mainTrans, endTimeTag))
				}
				if bgTrans != "" {
					lrcLines = append(lrcLines, fmt.Sprintf("%s%s%s", timeTag, bgTrans, endTimeTag))
				}
			}
		}
	}
	return strings.Join(lrcLines, "\n"), nil
}

// Word 模式处理逻辑
func conventWordTTMLToLineLRC(parsed *etree.Document) (string, error) {
	root := parsed.FindElement("tt")
	if root == nil {
		return "", errors.New("invalid TTML")
	}
	body := root.FindElement("body")
	if body == nil {
		return "", errors.New("invalid TTML")
	}
	translations := extractTranslations(root)
	var lines []string
	for _, div := range body.FindElements("div") {
		for _, p := range div.FindElements("p") {
			begin := p.SelectAttrValue("begin", "")
			end := p.SelectAttrValue("end", "")
			if begin == "" || end == "" {
				continue
			}
			start, err := parseLrcTime(begin)
			if err != nil {
				continue
			}
			finish, err := parseLrcTime(end)
			if err != nil {
				continue
			}
			main, background := getSplitText(p)
			main = strings.TrimSpace(main)
			background = strings.TrimSpace(background)
			if main != "" {
				lines = append(lines, fmt.Sprintf("[%s]%s[%s]", start, main, finish))
			}
			if background != "" {
				lines = append(lines, fmt.Sprintf("[%s]%s[%s]", start, background, finish))
			}
			if translated, ok := translations[p.SelectAttrValue("itunes:key", "")]; ok && translated != nil {
				translatedMain, translatedBackground := getSplitText(translated)
				translatedMain = strings.TrimSpace(translatedMain)
				translatedBackground = strings.TrimSpace(translatedBackground)
				if translatedMain != "" {
					lines = append(lines, fmt.Sprintf("[%s]%s[%s]", start, translatedMain, finish))
				}
				if translatedBackground != "" {
					lines = append(lines, fmt.Sprintf("[%s]%s[%s]", start, translatedBackground, finish))
				}
			}
		}
	}
	return strings.Join(lines, "\n"), nil
}

func conventSyllableTTMLToLRC(parsed *etree.Document) (string, error) {
	root := parsed.FindElement("tt")
	if root == nil {
		return "", errors.New("invalid TTML")
	}
	body := root.FindElement("body")
	if body == nil {
		return "", errors.New("invalid TTML")
	}

	var lrcLines []string
	isJa := root.SelectAttrValue("xml:lang", "") == "ja"
	isHant := root.SelectAttrValue("xml:lang", "") == "zh-Hant"

	translations := extractTranslations(root)
	transliterations := extractTransliterations(root)

	type LineContent struct {
		Text      string
		StartTime string
		EndTime   string
		HasTiming bool
	}

	// buildSplitLines 用于递归解析节点下的 span 结构，提取带时间戳的文本
	buildSplitLines := func(element *etree.Element) (main LineContent, bg LineContent) {
		var mainSyllables, bgSyllables []string
		var mainStart, mainEnd, bgStart, bgEnd string

		var processSpan func(e *etree.Element, isBgContext bool)
		processSpan = func(e *etree.Element, isBgContext bool) {
			sbegin := e.SelectAttrValue("begin", "")
			send := e.SelectAttrValue("end", "")
			text := e.Text()

			role := e.SelectAttrValue("ttm:role", "")
			isCurrentBg := isBgContext || (role == "x-bg")

			if sbegin != "" && send != "" && text != "" {
				st, _ := makeSyllableStamp(sbegin)
				et, _ := makeSyllableStamp(send)
				lrcTimeStart, _ := parseLrcTime(sbegin)

				item := fmt.Sprintf("%s%s%s", st, text, et)

				if isCurrentBg {
					bgSyllables = append(bgSyllables, item)
					if bgStart == "" {
						bgStart = lrcTimeStart
					}
					bgEnd = send
				} else {
					mainSyllables = append(mainSyllables, item)
					if mainStart == "" {
						mainStart = lrcTimeStart
					}
					mainEnd = send
				}
			} else {
				for _, child := range e.Child {
					switch c := child.(type) {
					case *etree.CharData:
						if shouldKeepText(c.Data) {
							if isCurrentBg {
								bgSyllables = append(bgSyllables, c.Data)
							} else {
								mainSyllables = append(mainSyllables, c.Data)
							}
						}
					case *etree.Element:
						processSpan(c, isCurrentBg)
					}
				}
			}
		}

		for _, child := range element.Child {
			switch c := child.(type) {
			case *etree.CharData:
				if shouldKeepText(c.Data) {
					mainSyllables = append(mainSyllables, c.Data)
				}
			case *etree.Element:
				processSpan(c, false)
			}
		}

		main.Text = strings.Join(mainSyllables, "")
		main.StartTime = mainStart
		main.HasTiming = mainStart != ""

		bg.Text = strings.Join(bgSyllables, "")
		bg.StartTime = bgStart
		bg.HasTiming = bgStart != ""

		if mainEnd != "" {
			mE, _ := parseLrcTime(mainEnd)
			main.EndTime = mE
		}
		if bgEnd != "" {
			bE, _ := parseLrcTime(bgEnd)
			bg.EndTime = bE
		}

		return
	}

	for _, div := range body.FindElements("div") {
		for _, p := range div.FindElements("p") {
			key := p.SelectAttrValue("itunes:key", "")
			pBegin := p.SelectAttrValue("begin", "")
			pEnd := p.SelectAttrValue("end", "")
			if pBegin == "" || pEnd == "" {
				continue
			}

			pBeginFmt, _ := parseLrcTime(pBegin)
			pEndFmt, _ := parseLrcTime(pEnd)

			// 解析原文
			mainContent, bgContent := buildSplitLines(p)

			if !mainContent.HasTiming {
				mainContent.StartTime = pBeginFmt
				mainContent.EndTime = pEndFmt
			}
			if !bgContent.HasTiming {
				bgContent.StartTime = pBeginFmt
				bgContent.EndTime = pEndFmt
			}

			// 基础翻译提取
			var mainTransStr, bgTransStr string
			trElem, hasTrans := translations[key]
			if hasTrans && trElem != nil {
				mainTransStr, bgTransStr = getSplitText(trElem)
			}

			// [Patch] 修复合并的翻译 (括号提取 - 保留括号)
			// 这里同样应用正则补丁
			if bgContent.Text != "" && bgTransStr == "" && mainTransStr != "" {
				// 检查中文括号
				if strings.HasSuffix(mainTransStr, "）") {
					idx := strings.LastIndex(mainTransStr, "（")
					if idx != -1 {
						// 修改：直接从 idx 开始截取，包含括号
						bgTransStr = strings.TrimSpace(mainTransStr[idx:])
						mainTransStr = strings.TrimSpace(mainTransStr[:idx])
					}
				} else if strings.HasSuffix(mainTransStr, ")") { // 检查英文括号
					idx := strings.LastIndex(mainTransStr, "(")
					if idx != -1 {
						// 修改：直接从 idx 开始截取，包含括号
						bgTransStr = strings.TrimSpace(mainTransStr[idx:])
						mainTransStr = strings.TrimSpace(mainTransStr[:idx])
					}
				}
			}

			// ================= 新增修改 (Word模式) =================
			mainTransStr = strings.ReplaceAll(mainTransStr, "（", "(")
			mainTransStr = strings.ReplaceAll(mainTransStr, "）", ")")
			bgTransStr = strings.ReplaceAll(bgTransStr, "（", "(")
			bgTransStr = strings.ReplaceAll(bgTransStr, "）", ")")
			// =====================================================

			// 确定辅助行（翻译/罗马音）的开始时间
			transStartTime := pBeginFmt
			if mainContent.StartTime != "" {
				transStartTime = mainContent.StartTime
			}

			// === 繁体中文逻辑 ===
			if isHant && mainTransStr != "" {
				transMainContent, transBgContent := buildSplitLines(trElem)

				// [修改] 繁体中文逻辑保留，但增加了一个判断：
				// 如果 XML 解析没有分出背景翻译(transBgContent为空)，但我们上面的补丁成功分出了(bgTransStr不为空)，
				// 那么我们不要进入这个复杂的时间回滚判断，或者我们使用补丁后的主唱翻译文本。

				// 场景：翻译有时候没有分离出 x-bg 标签，而是把背景人声直接拼在后面，但时间戳会发生回滚
				if transBgContent.Text == "" && transMainContent.Text != "" && transMainContent.HasTiming {
					// 定义临时结构用于解析
					type Syllable struct {
						FullText string
						StartStr string
						EndStr   string
					}
					var syllables []Syllable

					// 解析 transMainContent.Text (格式如 <00:00.00>文本<00:00.00>...)
					raw := transMainContent.Text
					validParse := true
					// 简单的字符串扫描解析
					for len(raw) > 0 {
						// 找开始时间标签 <...>
						idx1 := strings.Index(raw, "<")
						if idx1 == -1 {
							break
						}
						idx2 := strings.Index(raw[idx1:], ">")
						if idx2 == -1 {
							validParse = false
							break
						}
						idx2 += idx1
						startStr := raw[idx1+1 : idx2]

						// 找结束时间标签 <...>
						remaining := raw[idx2+1:]
						idx3 := strings.Index(remaining, "<") // 文本之后的那个 <
						if idx3 == -1 {
							validParse = false
							break
						}

						idx4 := strings.Index(remaining[idx3:], ">")
						if idx4 == -1 {
							validParse = false
							break
						}
						idx4 += idx3
						endStr := remaining[idx3+1 : idx4]

						// 截取完整片段
						absoluteEnd := idx2 + 1 + idx4 + 1
						fullItem := raw[idx1:absoluteEnd]

						syllables = append(syllables, Syllable{
							FullText: fullItem,
							StartStr: startStr,
							EndStr:   endStr,
						})

						if len(raw) >= absoluteEnd {
							raw = raw[absoluteEnd:]
						} else {
							raw = ""
						}
					}

					// 如果解析成功且有多个片段，开始检测时间回滚
					if validParse && len(syllables) > 1 {
						timeToSec := func(t string) float64 {
							var m, s, ms int
							fmt.Sscanf(t, "%d:%d.%d", &m, &s, &ms)
							return float64(m*60) + float64(s) + float64(ms)/100.0
						}

						splitIndex := -1
						var lastEndSec float64 = -1.0

						for i, s := range syllables {
							sStart := timeToSec(s.StartStr)
							sEnd := timeToSec(s.EndStr)

							// 核心判定
							if i > 0 && lastEndSec > 0 && sStart < (lastEndSec-0.2) {
								splitIndex = i
								break
							}

							// 更新最晚结束时间
							if sEnd > lastEndSec {
								lastEndSec = sEnd
							}
						}

						// 执行拆分
						if splitIndex > 0 {
							var mb, bb strings.Builder

							// 重建主唱部分
							for i := 0; i < splitIndex; i++ {
								mb.WriteString(syllables[i].FullText)
							}
							transMainContent.Text = mb.String()
							transMainContent.StartTime = syllables[0].StartStr
							transMainContent.EndTime = syllables[splitIndex-1].EndStr

							// 重建背景部分
							for i := splitIndex; i < len(syllables); i++ {
								bb.WriteString(syllables[i].FullText)
							}
							transBgContent.Text = bb.String()
							transBgContent.StartTime = syllables[splitIndex].StartStr
							transBgContent.EndTime = syllables[len(syllables)-1].EndStr
							transBgContent.HasTiming = true // 标记为有时间戳
						}
					}
				}

				finalText := transMainContent.Text
				// 如果 transMainContent (原始XML解析) 为空，或者原始解析没分出背景但我们的正则补丁分出了，
				// 则优先使用正则补丁处理后的 mainTransStr
				if finalText == "" || (transBgContent.Text == "" && bgTransStr != "") {
					finalText = mainTransStr
				}

				finalStart := transMainContent.StartTime
				if finalStart == "" {
					finalStart = transStartTime
				}
				finalEnd := transMainContent.EndTime
				if finalEnd == "" {
					finalEnd = pEndFmt
				}
				lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", finalStart, finalText, finalEnd))

				// 固定分离逻辑
				if transBgContent.Text != "" {
					ts := transBgContent.StartTime
					if ts == "" {
						ts = finalStart
					}
					te := transBgContent.EndTime
					if te == "" {
						te = finalEnd
					}
					lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", ts, transBgContent.Text, te))
				} else if bgTransStr != "" {
					ts := bgContent.StartTime
					if ts == "" {
						ts = pBeginFmt
					}
					te := bgContent.EndTime
					if te == "" {
						te = pEndFmt
					}
					lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", ts, bgTransStr, te))
				}
				continue
			}

			// === 标准流程 ===

			// 1. 输出原文主唱行
			if mainContent.Text != "" {
				ts := mainContent.StartTime
				if ts == "" {
					ts = pBeginFmt
				}
				te := mainContent.EndTime
				if te == "" {
					te = pEndFmt
				}
				lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", ts, mainContent.Text, te))
			}

			// 2. 输出罗马音行
			if isJa {
				if romajiElem, ok := transliterations[key]; ok && romajiElem != nil {
					rMain, rBg := buildSplitLines(romajiElem)

					// 主唱罗马音
					finalRomajiText := rMain.Text
					if finalRomajiText == "" {
						finalRomajiText, _ = getSplitText(romajiElem)
					}

					if finalRomajiText != "" {
						rs := rMain.StartTime
						if rs == "" {
							rs = transStartTime
						}
						re := rMain.EndTime
						if re == "" {
							re = pEndFmt
						}
						lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", rs, finalRomajiText, re))
					}

					// 背景罗马音 (固定分离)
					finalBgRomaji := rBg.Text
					if finalBgRomaji == "" {
						_, tempBg := getSplitText(romajiElem)
						finalBgRomaji = tempBg
					}

					if finalBgRomaji != "" {
						rs := rBg.StartTime
						if rs == "" {
							rs = transStartTime
						}
						re := rBg.EndTime
						if re == "" {
							re = pEndFmt
						}
						lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", rs, finalBgRomaji, re))
					}
				}
			}

			// 3. 输出翻译行
			if mainTransStr != "" {
				lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", transStartTime, mainTransStr, pEndFmt))
			}

			// 4. 输出背景人声行 (固定分离模式)
			if bgContent.Text != "" {
				ts := bgContent.StartTime
				if ts == "" {
					ts = pBeginFmt
				}
				te := bgContent.EndTime
				if te == "" {
					te = pEndFmt
				}
				lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", ts, bgContent.Text, te))

				if bgTransStr != "" {
					lrcLines = append(lrcLines, fmt.Sprintf("[%s]%s[%s]", ts, bgTransStr, te))
				}
			}
		}
	}

	return strings.Join(lrcLines, "\n"), nil
}

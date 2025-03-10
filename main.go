package main

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eiannone/keyboard"
	"github.com/mattn/go-runewidth"
)

// StationItem 儲存測站基本資料
type StationItem struct {
	StationID        string `json:"stationID"`
	StationName      string `json:"stationName"`
	CountryName      string `json:"countryName"`
	Area             string `json:"area"`
	StationStartDate string `json:"stationStartDate"`
	StationEndDate   string `json:"stationEndDate"`
}

// StationListResponse 用來解析測站列表 JSON
type StationListResponse struct {
	Data []struct {
		Item []StationItem `json:"item"`
	} `json:"data"`
}

// clearScreen 清除 CLI 畫面
func clearScreen() {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "cls")
	} else {
		cmd = exec.Command("clear")
	}
	cmd.Stdout = os.Stdout
	cmd.Run()
}

// centerText 根據字串實際顯示寬度進行居中填充
func centerText(text string, width int, fillchar string) string {
	textWidth := runewidth.StringWidth(text)
	if textWidth >= width {
		return text
	}
	paddingTotal := width - textWidth
	leftPadding := paddingTotal / 2
	rightPadding := paddingTotal - leftPadding
	return strings.Repeat(fillchar, leftPadding) + text + strings.Repeat(fillchar, rightPadding)
}

// getInputWithEsc 讀取鍵盤輸入，遇到 Esc 鍵返回取消標記
// 回傳值：輸入的字串、cancelled (true 表示按下 Esc)
func getInputWithEsc(prompt string) (string, bool) {
	fmt.Print(prompt)
	var input string
	for {
		char, key, err := keyboard.GetKey()
		if err != nil {
			fmt.Println("\n讀取鍵盤輸入錯誤：", err)
			return "", false
		}
		if key == keyboard.KeyEsc {
			fmt.Println() // 換行
			return "", true
		} else if key == keyboard.KeyEnter {
			fmt.Println() // 換行
			return input, false
		} else if key == keyboard.KeyBackspace || key == keyboard.KeyBackspace2 {
			if len(input) > 0 {
				_, size := utf8.DecodeLastRuneInString(input)
				input = input[:len(input)-size]
				// 從終端機刪除最後一個字元
				fmt.Print("\b \b")
			}
		} else if key == 0 { // 普通字元
			input += string(char)
			fmt.Print(string(char))
		}
	}
}

// parseFlexibleDate 嘗試以多種格式解析日期字串，並轉換為 time.Time
func parseFlexibleDate(dateStr string) (time.Time, error) {
	layouts := []string{
		"2006-01-02",
		"2006/01/02",
		"2006-1-2",
		"2006/1/2",
	}
	var t time.Time
	var err error
	for _, layout := range layouts {
		t, err = time.Parse(layout, dateStr)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("日期格式錯誤：%s", dateStr)
}

// getStationInfo 取得測站資訊並做過濾處理
func getStationInfo() ([]StationItem, error) {
	resp, err := http.Get("https://codis.cwa.gov.tw/api/station_list")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 新增：檢查 HTTP 狀態碼
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP GET error: %d", resp.StatusCode)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// 新增：檢查是否接收到 HTML 回應
	if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
		return nil, fmt.Errorf("收到 HTML 回應，可能發生錯誤：%s", string(body))
	}

	var slr StationListResponse
	err = json.Unmarshal(body, &slr)
	if err != nil {
		return nil, err
	}
	if len(slr.Data) < 2 {
		return nil, fmt.Errorf("JSON 資料格式不符")
	}
	items := slr.Data[1].Item

	// 過濾 stationID 以 "C0" 開頭且 stationEndDate 為空的項目
	var filtered []StationItem
	for _, item := range items {
		if strings.HasPrefix(item.StationID, "C0") && item.StationEndDate == "" {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

// getStationData 取得測站氣象資料，並依照各指標做資料拆解
func getStationData(stationID string, timeStart, timeEnd time.Time) ([]map[string]string, error) {
	urlStr := "https://codis.cwa.gov.tw/api/station"
	data := url.Values{}
	data.Set("type", "report_month")
	data.Set("stn_ID", stationID)
	data.Set("stn_type", "auto_C0")
	data.Set("more", "")
	data.Set("start", timeStart.Format("2006-01-02T15:04:05"))
	data.Set("end", timeEnd.Format("2006-01-02T15:04:05"))
	data.Set("item", "")

	var records []map[string]string
	retryCount := 3 // 最大重試次數
	waitDuration := time.Second // 初始等待時間

	for i := 0; i <= retryCount; i++ {
		resp, err := http.PostForm(urlStr, data)
		if err != nil {
			if i < retryCount {
				log.Printf("API 請求失敗 (嘗試 %d/%d): %v，等待 %v 後重試...", i+1, retryCount, err, waitDuration)
				time.Sleep(waitDuration)
				waitDuration *= 2 // 指數退避
				continue // 進行重試
			} else {
				return nil, fmt.Errorf("API 請求失敗 (已達最大重試次數): %w", err)
			}
		}
		defer resp.Body.Close()

		// 新增：檢查 HTTP 狀態碼
		if resp.StatusCode == http.StatusOK {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}

			// 新增：檢查是否接收到 HTML 回應
			if strings.HasPrefix(strings.TrimSpace(string(body)), "<") {
				return nil, fmt.Errorf("收到 HTML 回應，可能發生錯誤：%s", string(body))
			}

			var result map[string]interface{}
			err = json.Unmarshal(body, &result)
			if err != nil {
				return nil, err
			}

			// 取得 data[0]["dts"]
			dataArr, ok := result["data"].([]interface{})
			if !ok || len(dataArr) == 0 {
				return nil, fmt.Errorf("JSON 結構不符")
			}
			firstData, ok := dataArr[0].(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("JSON 結構不符")
			}
			dts, ok := firstData["dts"].([]interface{})
			if !ok {
				return nil, fmt.Errorf("JSON 中 dts 資料不符")
			}

			for _, rec := range dts {
				recMap, ok := rec.(map[string]interface{})
				if !ok {
					continue
				}
				record := make(map[string]string)
				// AirTemperature 處理
				if at, ok := recMap["AirTemperature"].(map[string]interface{}); ok {
					record["MaxAirTemperature"] = fmt.Sprintf("%v", at["Maximum"])
					record["MeanAirTemperature"] = fmt.Sprintf("%v", at["Mean"])
					record["MinAirTemperature"] = fmt.Sprintf("%v", at["Minimum"])
					record["MaxAirTemperatureTime"] = fmt.Sprintf("%v", at["MaximumTime"])
					record["MinAirTemperatureTime"] = fmt.Sprintf("%v", at["MinimumTime"])
				}
				// WindSpeed 與 WindDirection
				if ws, ok := recMap["WindSpeed"].(map[string]interface{}); ok {
					record["WindSpeed"] = fmt.Sprintf("%v", ws["Mean"])
				}
				if wd, ok := recMap["WindDirection"].(map[string]interface{}); ok {
					record["WindDirection"] = fmt.Sprintf("%v", wd["Prevailing"])
				}
				// StationPressure
				if sp, ok := recMap["StationPressure"].(map[string]interface{}); ok {
					record["MaxStationPressure"] = fmt.Sprintf("%v", sp["Maximum"])
					record["MinStationPressure"] = fmt.Sprintf("%v", sp["Minimum"])
					record["MeanStationPressure"] = fmt.Sprintf("%v", sp["Mean"])
					record["MaxStationPressureTime"] = fmt.Sprintf("%v", sp["MaximumTime"])
					record["MinStationPressureTime"] = fmt.Sprintf("%v", sp["MinimumTime"])
				}
				// RelativeHumidity
				if rh, ok := recMap["RelativeHumidity"].(map[string]interface{}); ok {
					record["MaxRelativeHumidity"] = fmt.Sprintf("%v", rh["Maximum"])
					record["MinRelativeHumidity"] = fmt.Sprintf("%v", rh["Minimum"])
					record["MeanRelativeHumidity"] = fmt.Sprintf("%v", rh["Mean"])
					record["MaxRelativeHumidityTime"] = fmt.Sprintf("%v", rh["MaximumTime"])
					record["MinRelativeHumidityTime"] = fmt.Sprintf("%v", rh["MinimumTime"])
				}
				// PeakGust
				if pg, ok := recMap["PeakGust"].(map[string]interface{}); ok {
					record["MaxPeakGust"] = fmt.Sprintf("%v", pg["Maximum"])
					record["MaxPeakGustTime"] = fmt.Sprintf("%v", pg["MaximumTime"])
					record["MaxPeakGustDirection"] = fmt.Sprintf("%v", pg["Direction"])
				}
				// Precipitation
				if pr, ok := recMap["Precipitation"].(map[string]interface{}); ok {
					record["AccumulationPrecipitation"] = fmt.Sprintf("%v", pr["Accumulation"])
					record["HourlyMaxPrecipitation"] = fmt.Sprintf("%v", pr["HourlyMaximum"])
					record["HourlyMaxPrecipitationTime"] = fmt.Sprintf("%v", pr["HourlyMaximumTime"])
					record["MeltFlagPrecipitation"] = fmt.Sprintf("%v", pr["MeltFlag"])
				}
				// SunshineDuration
				if sd, ok := recMap["SunshineDuration"].(map[string]interface{}); ok {
					record["SunshineDuration"] = fmt.Sprintf("%v", sd["Total"])
				}
				// GlobalSolarRadiation
				if gsr, ok := recMap["GlobalSolarRadiation"].(map[string]interface{}); ok {
					record["AccumulationGlobalSolarRadiation"] = fmt.Sprintf("%v", gsr["Accumulation"])
					record["HourlyMaximumGlobalSolarRadiation"] = fmt.Sprintf("%v", gsr["HourlyMaximum"])
					record["HourlyMaximumGlobalSolarRadiationTime"] = fmt.Sprintf("%v", gsr["HourlyMaximumTime"])
				}
				// 觀測日期
				if obs, ok := recMap["DataDate"]; ok {
					record["DataDate"] = fmt.Sprintf("%v", obs)
				}
				records = append(records, record)
			}
			// 請求成功，跳出重試迴圈
			break
		} else if resp.StatusCode >= http.StatusInternalServerError { // 5xx 錯誤
			if i < retryCount {
				log.Printf("API 伺服器錯誤 (%d) (嘗試 %d/%d)，等待 %v 後重試...", resp.StatusCode, i+1, retryCount, waitDuration)
				time.Sleep(waitDuration)
				waitDuration *= 2 // 指數退避
				continue // 進行重試
			} else {
				return nil, fmt.Errorf("API 伺服器錯誤 (%d) (已達最大重試次數)", resp.StatusCode)
			}
		} else { // 其他非 200 錯誤 (例如 4xx)
			return nil, fmt.Errorf("API 請求失敗，狀態碼: %d", resp.StatusCode) // 不重試
		}
	}

	return records, nil
}

var csvColumns = []string{
	"DataDate", "WindSpeed", "WindDirection", "SunshineDuration",
	"MaxAirTemperature", "MeanAirTemperature", "MinAirTemperature",
	"MaxAirTemperatureTime", "MinAirTemperatureTime",
	"MaxStationPressure", "MinStationPressure", "MeanStationPressure",
	"MaxStationPressureTime", "MinStationPressureTime",
	"MaxRelativeHumidity", "MinRelativeHumidity", "MeanRelativeHumidity",
	"MaxRelativeHumidityTime", "MinRelativeHumidityTime",
	"MaxPeakGust", "MaxPeakGustTime", "MaxPeakGustDirection",
	"AccumulationPrecipitation", "HourlyMaxPrecipitation", "HourlyMaxPrecipitationTime",
	"MeltFlagPrecipitation",
	"AccumulationGlobalSolarRadiation", "HourlyMaximumGlobalSolarRadiation", "HourlyMaximumGlobalSolarRadiationTime",
}

// writeCSV 將資料寫入 CSV 檔案
func writeCSV(filename string, records []map[string]string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// 寫入表頭
	if err := writer.Write(csvColumns); err != nil {
		return err
	}

	// 寫入每一筆記錄
	for _, rec := range records {
		row := make([]string, len(csvColumns))
		for i, col := range csvColumns {
			row[i] = rec[col]
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	return nil
}

// processStationData 處理測站資料的取得、儲存流程
func processStationData(stationID string, timeStart, timeEnd time.Time) error {
	var allRecords []map[string]string
	// 若查詢區間小於等於 366 天，直接呼叫 getStationData
	if timeEnd.Sub(timeStart).Hours() <= 366*24 {
		records, err := getStationData(stationID, timeStart, timeEnd)
		if err != nil {
			return fmt.Errorf("取得資料時發生錯誤：%w", err)
		}
		allRecords = records
	} else {
		// 若查詢區間大於 366 天，分段呼叫 getStationData，
		// 並檢查每段的起始日期是否大於今天，若大於則略過該段
		totalDays := int(timeEnd.Sub(timeStart).Hours() / 24)
		for i := 0; i < totalDays; i += 366 {
			startChunk := timeStart.AddDate(0, 0, i)
			// 若該區塊的起始日期大於今天，則略過
			if startChunk.After(time.Now()) {
				continue
			}
			endChunk := timeStart.AddDate(0, 0, i+366)
			if endChunk.After(timeEnd) {
				endChunk = timeEnd
			}
			fmt.Printf("取得區間：%s ~ %s\n", startChunk.Format("2006-01-02T15:04:05"), endChunk.Format("2006-01-02T15:04:05"))
			records, err := getStationData(stationID, startChunk, endChunk)
			if err != nil {
				return fmt.Errorf("取得資料時發生錯誤：%w", err)
			}
			fmt.Printf("取得 %d 筆資料，開始合併...\n", len(records))
			allRecords = append(allRecords, records...)
		}
	}
	filename := fmt.Sprintf("%s_%s_%s.csv", stationID, timeStart.Format("20060102"), timeEnd.Format("20060102"))
	if err := writeCSV(filename, allRecords); err != nil {
		return fmt.Errorf("儲存 CSV 時發生錯誤：%w", err)
	}
	fmt.Printf("氣象資料已儲存為 %s\n", filename)
	return nil
}

func main() {
	// 開啟鍵盤監聽
	if err := keyboard.Open(); err != nil {
		log.Fatal(err)
	}
	defer keyboard.Close()

	// 取得測站資訊
	stationInfo, err := getStationInfo()
	if err != nil {
		log.Fatal("取得測站資訊失敗：", err)
	}

	width := 60
	borderTop := "╒" + strings.Repeat("═", width) + "╕"
	line01 := "|" + centerText("CODiS 氣候觀測資料查詢系統爬蟲範例", width, " ") + "|"
	line02 := "|" + centerText("by: c55Math4833", width, " ") + "|"
	borderMiddle := "╞" + strings.Repeat("═", width) + "╡"
	line1 := "|" + centerText("請選擇要使用的功能：", width, " ") + "|"
	line2 := "|" + centerText("1. 手動輸入測站代碼取得氣象資料", width, " ") + "|"
	line3 := "|" + centerText("2. 循序查詢測站代碼取得氣象資料", width, " ") + "|"
	line4 := "|" + centerText("3. 退出程式", width, " ") + "|"
	borderBottom := "╘" + strings.Repeat("═", width) + "╛"

	reader := bufio.NewReader(os.Stdin)
MainLoop:
	for {
		clearScreen()
		fmt.Println(borderTop)
		fmt.Println(line01)
		fmt.Println(line02)
		fmt.Println(borderMiddle)
		fmt.Println(line1)
		fmt.Println(line2)
		fmt.Println(line3)
		fmt.Println(line4)
		fmt.Println(borderBottom)

		choice, cancelled := getInputWithEsc("請輸入您的選擇（按 Esc 退出程式）：")
		if cancelled || choice == "3" {
			fmt.Println("程式結束！")
			break MainLoop
		}
		if choice != "1" && choice != "2" {
			fmt.Println("無效的選擇，請重新輸入。")
			fmt.Println("按 Enter 繼續...")
			reader.ReadString('\n')
			continue MainLoop
		}

		// 選項 1：手動輸入測站代碼
		if choice == "1" {
		stationInput:
			for {
				clearScreen()
				stationID, cancelled := getInputWithEsc("請輸入測站代碼（按 Esc 返回主選單）：")
				if cancelled {
					break stationInput // 返回主選單
				}
				// 搜尋對應的測站資料
				var found *StationItem
				for i := range stationInfo {
					if stationInfo[i].StationID == stationID {
						found = &stationInfo[i]
						break
					}
				}
				if found == nil {
					fmt.Println("查無此測站代碼，請重新輸入！")
					fmt.Println("按 Enter 繼續...")
					reader.ReadString('\n')
					continue stationInput
				}
				clearScreen()
				fmt.Printf("您選擇的測站 %s - %s 位於 %s %s\n", found.StationID, found.StationName, found.CountryName, found.Area)

			startDateInput:
				var timeStart time.Time
				for {
					timeStartStr, cancelled := getInputWithEsc("請輸入查詢起始日期（格式：YYYY-MM-DD，按 Esc 返回上一步）：")
					if cancelled {
						// 返回到測站代碼輸入
						continue stationInput
					}
					t, err := parseFlexibleDate(timeStartStr)
					if err != nil {
						fmt.Println(err)
						fmt.Println("按 Enter 繼續...")
						reader.ReadString('\n')
						continue
					}
					// 檢查起始日期不得早於測站啟用日期
					stationStart, err := parseFlexibleDate(found.StationStartDate)
					if err == nil && t.Before(stationStart) {
						fmt.Printf("起始日期不得早於測站啟用日期：%s\n", found.StationStartDate)
						fmt.Println("按 Enter 重新輸入起始日期...")
						reader.ReadString('\n')
						continue
					}
					timeStart = t
					break
				}

				var timeEnd time.Time
				for {
					timeEndStr, cancelled := getInputWithEsc("請輸入查詢結束日期（格式：YYYY-MM-DD，按 Esc 返回上一步）：")
					if cancelled {
						// 返回到查詢起始日期
						goto startDateInput
					}
					t, err := parseFlexibleDate(timeEndStr)
					if err != nil {
						fmt.Println(err)
						fmt.Println("按 Enter 繼續...")
						reader.ReadString('\n')
						continue
					}
					if timeStart.After(t) {
						fmt.Println("開始日期不可大於結束日期")
						fmt.Println("按 Enter 重新輸入起始日期...")
						reader.ReadString('\n')
						goto startDateInput
					}
					timeEnd = t
					break
				}

				// 處理測站資料流程 (選項 1)
				if err := processStationData(stationID, timeStart, timeEnd); err != nil {
					fmt.Println("處理測站資料時發生錯誤：", err)
				}

				fmt.Println("按 Enter 繼續...")
				reader.ReadString('\n')
				break stationInput
			}
		} else if choice == "2" {
			// 選項 2：循序查詢測站代碼
		areaSelect:
			var area string
			for {
				clearScreen()
				fmt.Println("請選擇目標區域：")
				areaMap := make(map[string]bool)
				var uniqueAreas []string
				for _, s := range stationInfo {
					if !areaMap[s.Area] {
						areaMap[s.Area] = true
						uniqueAreas = append(uniqueAreas, s.Area)
					}
				}
				for i, a := range uniqueAreas {
					fmt.Printf("%d. %s\n", i+1, a)
				}
				areaIndexStr, cancelled := getInputWithEsc("請輸入區域代碼（按 Esc 返回主選單）：")
				if cancelled {
					// 返回主選單
					continue MainLoop
				}
				idx, err := strconv.Atoi(areaIndexStr)
				if err != nil || idx < 1 || idx > len(uniqueAreas) {
					fmt.Println("輸入錯誤，請重新輸入！")
					fmt.Println("按 Enter 繼續...")
					reader.ReadString('\n')
					continue
				}
				area = uniqueAreas[idx-1]
				fmt.Printf("您選擇的區域：%s\n", area)
				break
			}
			if area == "" {
				continue MainLoop
			}

		countySelect:
			var countryName string
			for {
				clearScreen()
				fmt.Printf("請選擇 %s 的縣市：\n", area)
				countyMap := make(map[string]bool)
				var uniqueCountries []string
				for _, s := range stationInfo {
					if s.Area == area && !countyMap[s.CountryName] {
						countyMap[s.CountryName] = true
						uniqueCountries = append(uniqueCountries, s.CountryName)
					}
				}
				for i, c := range uniqueCountries {
					fmt.Printf("%d. %s\n", i+1, c)
				}
				countryIndexStr, cancelled := getInputWithEsc("請輸入縣市代碼（按 Esc 返回上一步）：")
				if cancelled {
					// 返回到區域選擇
					goto areaSelect
				}
				idx, err := strconv.Atoi(countryIndexStr)
				if err != nil || idx < 1 || idx > len(uniqueCountries) {
					fmt.Println("輸入錯誤，請重新輸入！")
					fmt.Println("按 Enter 繼續...")
					reader.ReadString('\n')
					continue
				}
				countryName = uniqueCountries[idx-1]
				fmt.Printf("您選擇的縣市：%s\n", countryName)
				break
			}
			if countryName == "" {
				continue MainLoop
			}

		stationSelect:
			var stationID string
			var selectedStation *StationItem
			for {
				clearScreen()
				fmt.Println("以下為該區域的測站：")
				var filtered []StationItem
				for _, s := range stationInfo {
					if s.Area == area && s.CountryName == countryName {
						filtered = append(filtered, s)
					}
				}
				// 簡單表格格式輸出
				fmt.Printf("%-10s %-20s %-10s %-10s\n", "StationID", "StationName", "Country", "Area")
				for _, s := range filtered {
					fmt.Printf("%-10s %-20s %-10s %-10s\n", s.StationID, s.StationName, s.CountryName, s.Area)
				}
				stationID, cancelled := getInputWithEsc("請輸入測站代碼（按 Esc 返回上一步）：")
				if cancelled {
					// 返回到縣市選擇
					goto countySelect
				}
				for i := range filtered {
					if filtered[i].StationID == stationID {
						selectedStation = &filtered[i]
						break
					}
				}
				if selectedStation == nil {
					fmt.Println("查無此測站代碼，請重新輸入！")
					fmt.Println("按 Enter 繼續...")
					reader.ReadString('\n')
					continue
				}
				clearScreen()
				fmt.Printf("您選擇的測站 %s - %s 位於 %s %s\n", selectedStation.StationID, selectedStation.StationName, selectedStation.CountryName, selectedStation.Area)
				break
			}

		startDateSelect:
			var timeStart time.Time
			for {
				timeStartStr, cancelled := getInputWithEsc("請輸入查詢起始日期（格式：YYYY-MM-DD，按 Esc 返回上一步）：")
				if cancelled {
					// 返回到測站選擇
					goto stationSelect
				}
				t, err := parseFlexibleDate(timeStartStr)
				if err != nil {
					fmt.Println(err)
					fmt.Println("按 Enter 繼續...")
					reader.ReadString('\n')
					continue
				}
				// 檢查起始日期不得早於該測站的啟用日期
				stationStart, err := parseFlexibleDate(selectedStation.StationStartDate)
				if err == nil && t.Before(stationStart) {
					fmt.Printf("起始日期不得早於測站啟用日期：%s\n", selectedStation.StationStartDate)
					fmt.Println("按 Enter 重新輸入起始日期...")
					reader.ReadString('\n')
					continue
				}
				timeStart = t
				break
			}

			var timeEnd time.Time
			for {
				timeEndStr, cancelled := getInputWithEsc("請輸入查詢結束日期（格式：YYYY-MM-DD，按 Esc 返回上一步）：")
				if cancelled {
					// 返回到查詢起始日期
					goto startDateSelect
				}
				t, err := parseFlexibleDate(timeEndStr)
				if err != nil {
					fmt.Println(err)
					fmt.Println("按 Enter 繼續...")
					reader.ReadString('\n')
					continue
				}
				if timeStart.After(t) {
					fmt.Println("開始日期不可大於結束日期")
					fmt.Println("按 Enter 重新輸入起始日期...")
					reader.ReadString('\n')
					goto startDateSelect
				}
				timeEnd = t
				break
			}

			// 處理測站資料流程 (選項 2)
			if err := processStationData(stationID, timeStart, timeEnd); err != nil {
				fmt.Println("處理測站資料時發生錯誤：", err)
			}

			fmt.Println("按 Enter 繼續...")
			reader.ReadString('\n')
		}
	}
}
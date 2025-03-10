# CWA CODiS Crawler go

使用 Go 語言撰寫的交通部中央氣象署（CWA）[氣候觀測資料查詢服務（Climate Observation Data Inquire Service, CODiS）](https://codis.cwa.gov.tw/)自動測站數據擷取工具，於 Releases 已建置 Windows AMD64 封裝，若有其他環境需求可 go build 封裝至不同作業環境使用。

## 工具使用畫面
![CWA CODiS Crawler go](./img/fig_01.jpg)
![CWA CODiS Crawler go](./img/fig_02.jpg)
![CWA CODiS Crawler go](./img/fig_03.jpg)

## 輸出欄位
下載的 CSV 檔案包含以下欄位：  
而並非所有測站都有所有欄位的資料，有些欄位可能為空值。


|	欄位名稱	|	說明	|	欄位名稱	|	說明	|
|	:--:	|	:--:	|	:--:	|	:--:	|
|	DataDate	|	觀測時間(day)	|	MinStationPressureTime	|	測站最低氣壓時間(LST)	|
|	WindSpeed	|	風速(m/s)	|	MaxRelativeHumidity	|	最高相對濕度	|
|	WindDirection	|	風向(360degree)	|	MinRelativeHumidity	|	最低相對濕度	|
|	MaxAirTemperature	|	最高氣溫(℃)	|	MeanRelativeHumidity	|	相對溼度(%)	|
|	MeanAirTemperature	|	氣溫(℃)	|	MaxRelativeHumidityTime	|	最高相對濕度時間	|
|	MinAirTemperature	|	最低氣溫(℃)	|	MinRelativeHumidityTime	|	最小相對溼度時間(LST)	|
|	MaxAirTemperatureTime	|	最高氣溫時間(LST)	|	MaxPeakGust	|	最大瞬間風(m/s)	|
|	MinAirTemperatureTime	|	最低氣溫時間(LST)	|	MaxPeakGustTime	|	最大瞬間風風速時間(LST)	|
|	MaxStationPressure	|	測站最高氣壓(hPa)	|	MaxPeakGustDirection	|	最大瞬間風風向(360degree)	|
|	MinStationPressure	|	測站最低氣壓(hPa)	|	AccumulationPrecipitation	|	累積降水量(mm)	|
|	MeanStationPressure	|	測站氣壓(hPa)	|	HourlyMaxPrecipitation	|	最大時降水量(mm)	|
|	MaxStationPressureTime	|	測站最高氣壓時間(LST)	|	HourlyMaxPrecipitationTime	|	最大時降水量時間(LST)	|

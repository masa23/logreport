# logreport

LTSV形式およびJSON形式（ネスト不可）のログを収集・集計し、その結果をGraphiteに送信するツールです。



## 概要

logreportは、ログデータを収集・集計し、その結果をGraphiteプロトコルを使用してGraphiteサーバーに送信するためのツールです。主に、LTSV形式とJSON形式（ネスト不可）のログに対応しています。

## 対応形式

- LTSV形式
- JSON形式（ネスト不可）

## インストール

```bash
go install github.com/masa23/logreport/cmd/logreport@latest
```

または

```bash
git clone https://github.com/masa23/logreport.git
cd logreport
go build -o logreport ./cmd/logreport
```

## 使用方法

```bash
logreport -config config.yaml
```

### 設定ファイル例

```yaml
# デバッグモード
Debug: false

# PIDファイルのパス
PidFile: "/var/run/logreport.pid"

# エラーログファイルのパス
ErrorLogFile: "/var/log/logreport.log"

# ログバッファサイズ
LogBufferSize: 4096

# 読み込むログファイルのパス
LogFile: "/var/log/nginx/access.log"

# 読み込み位置を記録するファイルのパス
PosFile: "/var/log/nginx/access.log.pos"

# タイムスタンプのカラム名
TimeColumn: "time_local"

# タイムスタンプのフォーマット
TimeParse: "02/Jan/2006:15:04:05 -0700"

# ログフォーマット (ltsv または json)
LogFormat: ltsv

# レポート設定
Report:
  # 集計間隔
  Interval: 1s
  # 遅延時間
  Delay: 10s

# 出力先設定
Exporters:
  # Graphiteへの出力設定
  Graphite:
    Host: localhost
    Port: 2003
    Prefix: "sec.logreport.local"
    SendBuffer: 1000
    MaxRetryCount: 5
    RetryWait: 1s
  
  # OTLP/gRPCへの出力設定（オプション）
  #OtlpGrpc:
  #  URL: addrs:///localhost:4317
  #  TLS:
  #    Insecure: true
  #  SendBuffer: 1000
  #  MaxRetryCount: 10
  #  RetryWait: 1s
  #  ResourceAttributes:
  #    service.name: example-service
  #    server.address: localhost

# 収集するメトリクスの定義
Metrics:
  # ステータスコードのカウント（HTTP）
  - Description: "status count"
    ItemName: "http.status"
    Type: "itemCount"
    LogColumn: "status"
    Filter:
      - LogColumn: "scheme"
        Values:
          - "http"
        Bool: true

  # ステータスコードのカウント（HTTPS）
  - Description: "status count"
    ItemName: "https.status"
    Type: "itemCount"
    LogColumn: "status"
    Filter:
      - LogColumn: "scheme"
        Values:
          - "https"
        Bool: true

  # HTTPリクエスト数
  - Description: "http request count"
    ItemName: "http.request"
    Type: "count"
    LogColumn: "scheme"
    Filter:
      - LogColumn: "scheme"
        Values:
          - "http"
        Bool: true

  # HTTPSリクエスト数
  - Description: "https request count"
    ItemName: "https.request"
    Type: "count"
    LogColumn: "scheme"
    Filter:
      - LogColumn: "scheme"
        Values:
          - "https"
        Bool: true

  # キャッシュヒット数
  - Description: "hit counter"
    ItemName: "hit-count"
    Type: "itemCount"
    LogColumn: "upstream_cache_status"

  # 送信バイト数の合計
  - Description: "bytes sent"
    ItemName: "bytes_sent"
    Type: "sum"
    DataType: "int"
    LogColumn: "bytes_sent"

  # 最大アップストリームリクエスト時間
  - Description: "max upstream request time"
    ItemName: "upstream_request_time_max"
    Type: "max"
    DataType: "float"
    LogColumn: "upstream_request_time"

  # 最小アップストリームリクエスト時間
  - Description: "min upstream request time"
    ItemName: "upstream_request_time_min"
    Type: "min"
    DataType: "float"
    LogColumn: "upstream_request_time"
```

## ライセンス

MIT

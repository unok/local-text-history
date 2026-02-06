# File History Tracker - 実装計画

## 概要

特定フォルダ以下のテキストファイルの変更を検知し、SQLite に履歴保存。
Web UI でパス検索・差分表示・ダウンロードが可能。JetBrains の Local History に相当。

**技術スタック**: Go + 埋め込み SPA（単一バイナリ）

---

## アーキテクチャ

```
┌──────────────────────────────────────────────────────┐
│  file-history（単一バイナリ）                          │
│                                                      │
│  ┌────────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │ fsnotify   │→ │ Debounce │→ │ better-sqlite3   │ │
│  │ (inotify)  │  │ per-file │  │ WAL mode + zstd  │ │
│  └────────────┘  └──────────┘  └──────────────────┘ │
│                                       ↕              │
│  ┌────────────────────────────────────────────────┐  │
│  │ net/http + embed.FS                            │  │
│  │ ┌──────────┐  ┌───────────┐  ┌──────────────┐ │  │
│  │ │ REST API │  │ SPA (React│  │ diff計算     │ │  │
│  │ │          │  │  + Vite)  │  │ (go-diff)    │ │  │
│  │ └──────────┘  └───────────┘  └──────────────┘ │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

### 主要な設計判断

| 判断ポイント | 選択                            | 理由                                    |
| ------------ | ------------------------------- | --------------------------------------- |
| 保存方式     | 全文スナップショット + zstd圧縮 | 任意時点の復元が簡単、diff は表示時計算 |
| デバウンス   | ファイルごとに独立タイマー      | `Map<path, Timer>` でシンプル           |
| Web UI       | React SPA を `embed.FS` で同梱  | デプロイが単一バイナリで完結            |
| プロセス管理 | systemd                         | Ubuntu ネイティブ、自動再起動           |
| DB           | SQLite WAL モード               | 読み書き並行可能、運用が楽              |

---

## ディレクトリ構成

```
file-history/
├── cmd/
│   └── file-history/
│       └── main.go              # エントリポイント（CLI引数パース、起動）
├── internal/
│   ├── config/
│   │   └── config.go            # JSON設定の読み込み・デフォルト値
│   ├── db/
│   │   └── db.go                # SQLite操作（スキーマ・CRUD・zstd圧縮/解凍）
│   ├── watcher/
│   │   └── watcher.go           # fsnotify + デバウンス + 保存トリガー
│   ├── server/
│   │   └── server.go            # HTTP API + 静的ファイル配信
│   └── diff/
│       └── diff.go              # 2つのスナップショット間の差分計算
├── web/                         # React SPA（Vite）
│   ├── src/
│   │   ├── App.tsx
│   │   ├── components/
│   │   │   ├── FileSearch.tsx    # パス検索UI
│   │   │   ├── SnapshotList.tsx  # スナップショット一覧
│   │   │   ├── DiffView.tsx      # 差分表示（diff2html）
│   │   │   └── Layout.tsx        # 共通レイアウト
│   │   └── lib/
│   │       └── api.ts            # API クライアント
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
├── go.mod
├── go.sum
├── Makefile                     # ビルド（web build → go embed → go build）
├── config.example.json
└── file-history.service         # systemd ユニットファイル
```

---

## DB スキーマ

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;

CREATE TABLE files (
  id       INTEGER PRIMARY KEY AUTOINCREMENT,
  path     TEXT NOT NULL UNIQUE,
  created  INTEGER NOT NULL DEFAULT (unixepoch()),
  updated  INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE snapshots (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  file_id   INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  content   BLOB NOT NULL,          -- zstd 圧縮済み全文
  size      INTEGER NOT NULL,       -- 元のサイズ（バイト）
  hash      TEXT NOT NULL,          -- SHA-256（重複スキップ用）
  timestamp INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX idx_snapshots_file_ts ON snapshots(file_id, timestamp DESC);
CREATE INDEX idx_files_path ON files(path);
```

**重複スキップ**: 保存前に直前スナップショットの hash と比較し、同一なら保存しない。

---

## REST API 設計

| メソッド | パス                                 | 説明                             |
| -------- | ------------------------------------ | -------------------------------- |
| GET      | `/api/files?q=xxx&limit=20&offset=0` | パス部分一致検索                 |
| GET      | `/api/files/:id`                     | ファイル詳細                     |
| GET      | `/api/files/:id/snapshots`           | スナップショット一覧             |
| GET      | `/api/snapshots/:id`                 | スナップショット内容取得         |
| GET      | `/api/snapshots/:id/download`        | 生ファイルダウンロード           |
| GET      | `/api/diff?from=:id&to=:id`          | 2スナップショット間の差分        |
| GET      | `/api/stats`                         | 統計情報                         |
| DELETE   | `/api/files/:id`                     | ファイルと全スナップショット削除 |
| `*`      | `/*`                                 | SPA フォールバック（embed.FS）   |

---

## 設定ファイル（config.json）

```json
{
  "watchDirs": ["/home/user/projects"],
  "debounceSec": 2,
  "port": 9876,
  "dbPath": "~/.local/share/file-history/history.db",
  "extensions": [
    ".go",
    ".ts",
    ".tsx",
    ".js",
    ".jsx",
    ".py",
    ".rs",
    ".java",
    ".kt",
    ".c",
    ".cpp",
    ".h",
    ".html",
    ".css",
    ".scss",
    ".vue",
    ".svelte",
    ".json",
    ".yaml",
    ".yml",
    ".toml",
    ".xml",
    ".md",
    ".txt",
    ".sh",
    ".sql",
    ".graphql",
    ".proto"
  ],
  "excludePatterns": [
    "**/node_modules/**",
    "**/.git/**",
    "**/vendor/**",
    "**/dist/**",
    "**/build/**",
    "**/.next/**",
    "**/__pycache__/**",
    "**/target/**",
    "**/*.min.js",
    "**/*.min.css",
    "**/*.lock",
    "**/package-lock.json",
    "**/pnpm-lock.yaml"
  ],
  "maxFileSize": 1048576,
  "maxSnapshots": 0
}
```

---

## 依存ライブラリ

### Go

| パッケージ                           | 用途                             |
| ------------------------------------ | -------------------------------- |
| `github.com/fsnotify/fsnotify`       | inotify ラッパー（ファイル監視） |
| `github.com/mattn/go-sqlite3`        | SQLite ドライバ（CGO）           |
| `github.com/klauspost/compress/zstd` | zstd 圧縮/解凍                   |
| `github.com/sergi/go-diff`           | 差分計算（unified diff 生成）    |
| 標準ライブラリ `net/http`, `embed`   | HTTPサーバー + SPA埋め込み       |

### Web（React SPA）

| パッケージ              | 用途                                       |
| ----------------------- | ------------------------------------------ |
| `react` + `react-dom`   | UI フレームワーク                          |
| `diff2html`             | diff のリッチ表示（side-by-side / inline） |
| `@tanstack/react-query` | APIデータ取得・キャッシュ                  |
| `tailwindcss`           | スタイリング                               |

---

## 実装フェーズ

### Phase 1: コア（Watcher + DB）

1. `internal/config` — JSON 設定読み込み・デフォルト値・バリデーション
2. `internal/db` — SQLite スキーマ作成、スナップショット CRUD、zstd 圧縮/解凍
3. `internal/watcher` — fsnotify 初期化、再帰的ディレクトリ登録、デバウンスタイマー
4. `cmd/file-history/main.go` — CLI 引数パース、watcher + server 起動
5. 動作確認: ファイル変更 → DB にスナップショットが保存されることを確認

### Phase 2: API サーバー

6. `internal/diff` — go-diff を使った unified diff 生成
7. `internal/server` — REST API 全エンドポイント実装
8. API テスト: curl で全エンドポイントの動作確認

### Phase 3: Web UI

9. Vite + React プロジェクト初期化（web/）
10. `FileSearch` — パス検索バー + 結果一覧表示
11. `SnapshotList` — ファイル選択時のスナップショットタイムライン
12. `DiffView` — diff2html による差分表示（side-by-side / inline 切替）
13. ダウンロードボタン（任意スナップショットの生ファイル取得）

### Phase 4: ビルド・デプロイ

14. `Makefile` — `web build` → `go build`（embed.FS 込み）でシングルバイナリ生成
15. `file-history.service` — systemd ユニットファイル
16. README.md — インストール手順・使い方

---

## ビルド手順

```makefile
.PHONY: build clean dev

# Web UI ビルド → Go バイナリに埋め込んでビルド
build:
	cd web && npm ci && npm run build
	CGO_ENABLED=1 go build -o bin/file-history ./cmd/file-history

# 開発用（web は vite dev, Go は air でホットリロード）
dev:
	cd web && npm run dev &
	air -c .air.toml

clean:
	rm -rf bin/ web/dist/
```

---

## systemd ユニットファイル

```ini
[Unit]
Description=File History Tracker
After=network.target

[Service]
Type=simple
User=%i
ExecStart=/usr/local/bin/file-history --config /etc/file-history/config.json
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

---

## 将来の拡張案（スコープ外）

- タイムライン表示（全ファイルの変更を時系列で横断表示）
- 統計ダッシュボード（変更頻度、ファイルサイズ推移）
- CLI での差分表示（`file-history diff <path>`）
- スナップショットの自動パージ（30日以上前は日次に間引き等）
- 複数マシン間の同期

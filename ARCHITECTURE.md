# Architecture

File History Tracker の内部構造に関する開発者向けドキュメントです。

## 全体アーキテクチャ

```
┌──────────────────────────────────────────────────────┐
│  file-history（単一バイナリ）                          │
│                                                      │
│  ┌────────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │ fsnotify   │→ │ Debounce │→ │ SQLite           │ │
│  │ (inotify)  │  │ per-file │  │ WAL mode + zstd  │ │
│  └────────────┘  └──────────┘  └──────────────────┘ │
│                                       ↕              │
│  ┌────────────────────────────────────────────────┐  │
│  │ net/http + embed.FS                            │  │
│  │ ┌──────────┐  ┌───────────┐  ┌──────────────┐ │  │
│  │ │ REST API │  │ SPA (React│  │ diff 計算    │ │  │
│  │ │ + SSE    │  │  + Vite)  │  │ (go-diff)    │ │  │
│  │ └──────────┘  └───────────┘  └──────────────┘ │  │
│  └────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────┘
```

1. **fsnotify** がファイル変更イベント（Write / Create / Rename）を検知
2. **Debounce** がファイルごとに独立したタイマーで短時間の連続変更をまとめる
3. **DB 書き込み** で zstd 圧縮した全文スナップショットを SQLite に保存（バッチ書き込み対応）
4. **SSE** で接続中のブラウザに変更を通知
5. **REST API** で SPA から履歴検索・差分表示・ダウンロードを提供

## ディレクトリ構成

```
local-text-history/
├── cmd/
│   └── file-history/
│       └── main.go              # エントリポイント（CLI 引数パース、起動）
├── internal/
│   ├── config/
│   │   ├── config.go            # JSON 設定の読み込み・デフォルト値・バリデーション
│   │   └── config_test.go
│   ├── db/
│   │   ├── db.go                # SQLite 操作（スキーマ・CRUD・zstd 圧縮/解凍・マイグレーション）
│   │   └── db_test.go
│   ├── diff/
│   │   ├── diff.go              # unified diff 生成（go-diff ベース）
│   │   └── diff_test.go
│   ├── server/
│   │   ├── server.go            # HTTP API + SSE + SPA 配信 + Basic 認証
│   │   └── server_test.go
│   └── watcher/
│       ├── watcher.go           # fsnotify イベントループ・デバウンス・リネーム検知・バッチ保存
│       ├── filter.go            # 拡張子フィルタ・除外パターン判定・バイナリ判定
│       ├── scanner.go           # 新規ディレクトリの既存ファイルスキャン
│       └── watcher_test.go
├── web/
│   ├── embed.go                 # go:embed ディレクティブ（dist/ を埋め込み）
│   ├── src/
│   │   ├── App.tsx              # ルート: ルーティング・SSE 接続
│   │   ├── main.tsx             # React エントリポイント
│   │   ├── components/
│   │   │   ├── Dashboard.tsx    # 履歴フィード + 検索
│   │   │   ├── FilePage.tsx     # ファイル詳細・スナップショット一覧・差分表示
│   │   │   ├── DiffView.tsx     # diff2html による差分レンダリング
│   │   │   └── Layout.tsx       # 共通レイアウト・DB ダウンロードボタン
│   │   ├── lib/
│   │   │   ├── api.ts           # API クライアント（React Query フック）
│   │   │   ├── api.test.ts
│   │   │   ├── format.ts        # 表示フォーマット用ユーティリティ
│   │   │   ├── format.test.ts
│   │   │   └── router.ts        # SPA ルーティング（History API ベース）
│   │   ├── styles/
│   │   └── index.css
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   └── tsconfig.json
├── .github/
│   ├── workflows/
│   │   └── ci.yml               # CI: Go テスト、Web テスト、フルビルド
│   └── dependabot.yml           # Dependabot: gomod, npm, github-actions
├── go.mod
├── go.sum
├── Makefile                     # ビルド: web build → go build
├── config.example.json
└── file-history.service         # systemd ユーザーモードユニットファイル
```

## 主要な設計判断

| 判断ポイント | 選択 | 理由 |
|-------------|------|------|
| 保存方式 | 全文スナップショット + zstd 圧縮 | 任意時点の復元が簡単。diff は表示時に計算 |
| 重複スキップ | SHA-256 ハッシュ比較 | 直前スナップショットと同一なら保存しない |
| デバウンス | ファイルごとに独立タイマー | `Map<path, Timer>` でシンプル。連続変更をまとめる |
| DB 書き込み | バッチ書き込み + リトライ | 複数ファイルを1トランザクションで保存。`database is locked` 時に自動リトライ |
| Web UI | React SPA を `embed.FS` で同梱 | デプロイが単一バイナリで完結 |
| SPA ルーティング | History API ベース（自前実装） | 軽量。`useSyncExternalStore` で React と統合 |
| プロセス管理 | systemd ユーザーモード | root 権限不要。`WantedBy=default.target` |
| DB | SQLite WAL モード | 読み書き並行可能、運用が楽 |
| PK | UUIDv7（TEXT 型） | 時系列ソート可能な UUID。旧 INTEGER PK からの自動マイグレーション対応 |
| リネーム検知 | Rename + Create イベントのペアリング | fsnotify の Rename イベント後 500ms 以内に Create があれば対として記録 |

## DB スキーマ

3つのテーブルで構成されます。全テーブルの主キーは UUIDv7（TEXT 型）です。

### files

```sql
CREATE TABLE files (
    id       TEXT PRIMARY KEY,
    path     TEXT NOT NULL UNIQUE,
    created  INTEGER NOT NULL DEFAULT (unixepoch()),
    updated  INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_files_path ON files(path);
```

### snapshots

```sql
CREATE TABLE snapshots (
    id        TEXT PRIMARY KEY,
    file_id   TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    content   BLOB NOT NULL,          -- zstd 圧縮済み全文
    size      INTEGER NOT NULL,       -- 元のサイズ（バイト）
    hash      TEXT NOT NULL,          -- SHA-256（重複スキップ用）
    timestamp INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_snapshots_file_ts ON snapshots(file_id, timestamp DESC);
CREATE INDEX idx_snapshots_timestamp ON snapshots(timestamp DESC, id DESC);
```

### renames

```sql
CREATE TABLE renames (
    id          TEXT PRIMARY KEY,
    old_file_id TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    new_file_id TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
    old_path    TEXT NOT NULL,
    new_path    TEXT NOT NULL,
    timestamp   INTEGER NOT NULL DEFAULT (unixepoch())
);
CREATE INDEX idx_renames_old_file ON renames(old_file_id, timestamp DESC);
CREATE INDEX idx_renames_new_file ON renames(new_file_id, timestamp DESC);
```

### マイグレーション

旧スキーマ（`INTEGER PRIMARY KEY`）から新スキーマ（`TEXT PRIMARY KEY` / UUIDv7）への自動マイグレーションが起動時に実行されます。`PRAGMA table_info` で `id` カラムの型を確認し、INTEGER であれば新テーブルへデータを移行します。

## 依存ライブラリ

### Go

| パッケージ | 用途 |
|-----------|------|
| `github.com/fsnotify/fsnotify` | inotify ラッパー（ファイル監視） |
| `github.com/mattn/go-sqlite3` | SQLite ドライバ（CGO） |
| `github.com/klauspost/compress/zstd` | zstd 圧縮/解凍 |
| `github.com/sergi/go-diff` | 差分計算（unified diff 生成） |
| `github.com/bmatcuk/doublestar/v4` | glob パターンマッチング（除外パターン用） |
| `github.com/google/uuid` | UUIDv7 生成 |
| `golang.org/x/sys` | システムコール（ディスク容量チェック用） |

### Web

| パッケージ | 用途 |
|-----------|------|
| `react`, `react-dom` | UI フレームワーク |
| `@tanstack/react-query` | API データ取得・キャッシュ |
| `diff2html` | diff のリッチ表示（side-by-side / inline） |
| `tailwindcss` | スタイリング |
| `vite` | 開発サーバー・ビルド |
| `vitest` | テスト |
| `typescript` | 型チェック |

## CI / CD

### GitHub Actions（`.github/workflows/ci.yml`）

3つのジョブで構成:

1. **Go Tests**: `go vet` + `go test -race ./...`
2. **Web Tests**: `npm ci` → `npm run test` → `npm run build`（型チェック含む）
3. **Full Build**: Go Tests と Web Tests が成功した後に `make build` でフルビルド

### Dependabot（`.github/dependabot.yml`）

| エコシステム | ディレクトリ | 頻度 |
|-------------|------------|------|
| gomod | `/` | weekly |
| npm | `/web` | weekly |
| github-actions | `/` | weekly |

## テスト

### Go テスト

```bash
CGO_ENABLED=1 go test ./...
# または
make test
```

テスト対象パッケージ:
- `internal/config` — 設定読み込み・バリデーション
- `internal/db` — DB 操作・マイグレーション
- `internal/diff` — unified diff 生成
- `internal/server` — HTTP API エンドポイント
- `internal/watcher` — ファイル監視・フィルタ・デバウンス

### Web テスト

```bash
cd web && npm test
```

vitest で `api.test.ts`, `format.test.ts` を実行します。

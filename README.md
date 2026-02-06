# File History Tracker

JetBrains Local History 相当のファイル履歴追跡ツール。指定ディレクトリ内のテキストファイルの変更を検知し、SQLite にスナップショットとして保存。Web UI でパス検索・差分表示・ダウンロードが可能な単一バイナリ。

## 機能

- **ファイル監視**: fsnotify によるリアルタイムファイル変更検知（起動後の新規ファイルも自動検知）
- **スナップショット保存**: zstd 圧縮 + SHA-256 重複スキップ付き SQLite(WAL モード)
- **バイナリ判定**: NUL バイト方式で自動判定し、バイナリファイルは監視対象から除外
- **リアルタイム通知**: SSE（Server-Sent Events）で変更をブラウザにプッシュ
- **Web UI**: 検出履歴フィード、ファイル検索、スナップショットタイムライン、1-click 差分表示(side-by-side / inline)
- **単一バイナリ**: Go embed で React SPA を同梱

## 必要環境

- Go 1.25+（CGO 有効）
- Node.js 20+
- GCC（sqlite3 ビルド用）

## ビルド

```bash
make build
```

生成バイナリ: `bin/file-history`

## 使い方

### 1. 設定ファイルの作成

```bash
cp config.example.json ~/.config/file-history/config.json
```

`watchDirs` を監視したいディレクトリに変更してください。

### 2. 起動

```bash
./bin/file-history --config ~/.config/file-history/config.json
```

ブラウザで `http://localhost:9876` を開きます。

### 3. systemd で自動起動（ユーザモード）

```bash
cp file-history.service ~/.config/systemd/user/
systemctl --user enable --now file-history
```

## 設定項目

| 項目 | 型 | デフォルト | 説明 |
|------|------|-----------|------|
| `watchDirs` | `string[]` | (必須) | 監視するディレクトリ |
| `debounceSec` | `int` | `2` | デバウンス秒数（ファイルごと独立） |
| `port` | `int` | `9876` | HTTP サーバーポート |
| `dbPath` | `string` | `~/.local/share/file-history/history.db` | SQLite データベースパス |
| `extensions` | `string[]` | (未指定) | 監視対象の拡張子。未指定時はバイナリ判定のみで全テキストファイルを監視 |
| `excludePatterns` | `string[]` | (下記参照) | 除外パターン（`**` 対応） |
| `maxFileSize` | `int` | `1048576` | 最大ファイルサイズ（バイト） |
| `maxSnapshots` | `int` | `0` | ファイルあたり最大スナップショット数（0=無制限） |

### excludePatterns のデフォルト値

`excludePatterns` 未指定時は以下が自動適用されます:

`**/node_modules/**`, `**/.git/**`, `**/vendor/**`, `**/dist/**`, `**/build/**`, `**/.next/**`, `**/__pycache__/**`, `**/target/**`, `**/*.min.js`, `**/*.min.css`, `**/*.lock`, `**/package-lock.json`, `**/pnpm-lock.yaml`

## API

| メソッド | パス | 説明 |
|----------|------|------|
| GET | `/api/history?limit=50` | 直近の変更検出一覧（トップ画面用） |
| GET | `/api/events` | SSE ストリーム（リアルタイム変更通知） |
| GET | `/api/files?q=xxx&limit=20&offset=0` | パス部分一致検索（`q` 空で全ファイルを更新日時順に返す） |
| GET | `/api/files/:id` | ファイル詳細 |
| GET | `/api/files/:id/snapshots` | スナップショット一覧 |
| GET | `/api/snapshots/:id` | スナップショット内容取得 |
| GET | `/api/snapshots/:id/download` | 生ファイルダウンロード |
| GET | `/api/diff?from=:id&to=:id` | 2 スナップショット間の差分（`from` 省略で空内容との差分） |
| GET | `/api/stats` | 統計情報 |
| DELETE | `/api/files/:id` | ファイルと全スナップショット削除 |

## 開発

```bash
# Web UI 開発サーバー（Vite proxy で Go サーバーに転送）
cd web && npm run dev

# Go サーバー起動
go run ./cmd/file-history --config config.example.json

# テスト実行
make test
```

## 技術スタック

- **バックエンド**: Go, fsnotify, SQLite (WAL), zstd, go-diff, UUIDv7
- **フロントエンド**: React, TypeScript, TailwindCSS, @tanstack/react-query, diff2html, Vite, SSE (EventSource)

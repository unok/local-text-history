# API リファレンス

| メソッド | パス | 説明 |
|----------|------|------|
| GET | `/api/history?limit=50&offset=0&q=xxx` | 直近の変更検出一覧（スナップショット + リネーム）。`q` でパス部分一致検索 |
| GET | `/api/events` | SSE ストリーム（リアルタイム変更通知） |
| GET | `/api/files?q=xxx&limit=20&offset=0` | ファイル検索。`q` 空で全ファイルを更新日時順に返す |
| GET | `/api/files/:id` | ファイル詳細 |
| GET | `/api/files/:id/snapshots` | スナップショット一覧 |
| GET | `/api/files/:id/renames` | リネーム履歴 |
| GET | `/api/snapshots/:id` | スナップショット内容取得 |
| GET | `/api/snapshots/:id/download` | 生ファイルダウンロード |
| GET | `/api/diff?from=:id&to=:id` | 2 スナップショット間の差分（`from` 省略で空内容との差分） |
| GET | `/api/stats` | 統計情報（ファイル数、スナップショット数、合計サイズ、監視ディレクトリ） |
| GET | `/api/database/download` | データベースダウンロード |
| DELETE | `/api/files/:id` | ファイルと全スナップショットの削除 |

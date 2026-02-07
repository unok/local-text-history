# Security Review: Basic認証の追加

## レビュー対象
- `internal/config/config.go` - BasicAuthConfig構造体とバリデーション追加
- `internal/server/server.go` - Basic認証ミドルウェア実装
- `config.example.json` - サンプル設定にbasicAuth追加
- `cmd/file-history/main.go` - BasicAuth設定の受け渡し
- `internal/config/config_test.go` - 設定バリデーションテスト
- `internal/server/server_test.go` - 認証テスト

## 判定: APPROVE

## セキュリティ分析

### 1. タイミング攻撃対策
**リスク: なし**

認証情報の比較に `crypto/subtle.ConstantTimeCompare` を使用しており、タイミングサイドチャネル攻撃に対して安全です。username と password の両方に定数時間比較が適用されています。

```go
subtle.ConstantTimeCompare([]byte(username), []byte(s.basicAuth.Username)) != 1 ||
subtle.ConstantTimeCompare([]byte(password), []byte(s.basicAuth.Password)) != 1
```

### 2. 認証バイパス
**リスク: なし**

- `basicAuth == nil` の場合のみ認証をスキップ（要件通り）
- ミドルウェアは `Handler()` メソッドで全ハンドラーをラップしており、個別ルートの認証漏れなし
- `r.BasicAuth()` の戻り値 `ok` を確認し、Authorization ヘッダーの欠如を検出

```go
func (s *Server) Handler() http.Handler {
    if s.basicAuth == nil {
        return s.mux
    }
    return s.basicAuthMiddleware(s.mux)
}
```

### 3. 設定バリデーション
**リスク: なし**

- `basicAuth` 指定時は username/password 両方が必須
- 空文字のusername/passwordを拒否
- `basicAuth` 省略時は `nil` で認証なし動作（正しいオプショナル実装）

### 4. 機密情報の取り扱い
**リスク: なし**

| チェック項目 | 結果 |
|-------------|------|
| config.json が .gitignore に含まれる | PASS - `config.json` はgitignoreに登録済み |
| 認証失敗時のログ漏洩 | PASS - "unauthorized" のみ出力、認証情報をログに記録しない |
| config.example.json のサンプル値 | PASS - `"changeme"` は変更を促す適切なプレースホルダー |
| 500エラー時の内部情報隠蔽 | PASS - `writeError` は500系で "internal server error" に置換 |

### 5. インジェクション攻撃
**リスク: なし**

- 認証変更にSQL操作は含まれない
- 外部コマンド実行は含まれない
- エラーレスポンスはJSON形式で返却、XSSリスクなし
- `WWW-Authenticate` ヘッダーの realm 値はハードコードされた文字列リテラル

### 6. HTTPS/TLS
**リスク: 低（INFO）**

Basic認証の認証情報はBase64エンコードのみで送信されます。TLS非使用時は傍受リスクがありますが、ローカルファイル履歴管理ツールとしての用途を考慮すると許容範囲です。リモートアクセスが必要な場合はリバースプロキシでTLS終端を推奨します。

### 7. ブルートフォース対策
**リスク: 低（INFO）**

認証失敗時のレート制限・アカウントロックは未実装です。ローカルツールとしては過剰な対策であり、許容範囲と判断します。

### 8. テストカバレッジ
**リスク: なし**

| テストケース | 結果 |
|-------------|------|
| 認証情報なしのリクエスト拒否 | PASS |
| 誤った認証情報の拒否 | PASS |
| 正しい認証情報の受入 | PASS |
| basicAuth未設定時のスキップ | PASS |
| 設定バリデーション（有効/空username/空password/省略） | PASS |

### 9. 依存関係
**リスク: なし**

- `crypto/subtle` は Go 標準ライブラリ。新しい外部依存パッケージの追加なし
- `config` パッケージへの依存は内部パッケージ間の正当な参照

## 総合評価

この変更はセキュリティ上の問題を導入していません。以下の点が特に優れています：

1. **`crypto/subtle.ConstantTimeCompare`** によるタイミング攻撃対策
2. **全ルート一括保護**のミドルウェアアーキテクチャ（個別ルートの保護漏れリスクなし）
3. **適切なバリデーション**（部分設定の拒否、空文字の拒否）
4. **情報漏洩防止**（認証情報をログに出力しない、500エラーの内部情報隠蔽）
5. **網羅的なテスト**（正常系・異常系・未設定時の全パターン）

ローカルファイル履歴管理ツールとしてのBasic認証実装は、セキュリティ要件を十分に満たしています。

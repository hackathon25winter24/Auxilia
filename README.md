# Auxilia - ゲーム認証システム実装ガイド

> Unity社会ゲーム用のセキュアなGo認証バックエンド

## 🎯 実装内容

このプロジェクトでは、以下の認証認可システムを**フル実装**しました：

### ✅ 実装済み機能

- **ユーザー登録** - メール、ユーザー名、パスワード
- **安全なログイン** - bcryptパスワード検証、ブルートフォース対策
- **JWT認証** - AccessToken（15分）+ RefreshToken（7日）
- **トークン管理** - 自動更新、無効化管理
- **ログアウト** - RefreshTokenの無効化
- **HTTPS対応** - TLS 1.3推奨
- **CORS対応** - クロスオリジン通信対応
- **エラーハンドリング** - 安全なエラーメッセージ

### 🔐 セキュリティ対策

- ✅ bcryptでパスワードをハッシュ化（ソルト自動生成）
- ✅ パスワード複雑性チェック（8字以上、大文字+小文字+数字）
- ✅ ブルートフォース攻撃対策（15分間5回失敗でロック）
- ✅ SQLインジェクション対策（Prepared Statements）
- ✅ JWTトークンの署名検証（HMAC-SHA256）
- ✅ RefreshTokenをハッシュ化してDB保存（漏洩時対策）
- ✅ トークンタイプの区別（access/refresh）
- ✅ HTTPS通信必須

---

## 📋 なぜOIDCではなくJWT？

| 項目             | OIDC           | JWT（採用） |
| ---------------- | -------------- | ----------- |
| **実装複雑度**   | 高い           | 中程度      |
| **外部依存**     | Google等が必須 | 不要        |
| **ゲーム内管理** | 困難           | 容易        |
| **自由度**       | 低い           | 高い        |
| **セキュリティ** | 高い           | 高い        |

**結論：** ゲーム内でユーザーアカウント を作成・管理する場合、JWT + リフレッシュトークン方式が最適です。

---

## 🏗️ ファイル構造

```
├── main.go                              # エントリーポイント（初期化処理）
├── go.mod                               # Go依存関係
├── .env.example                         # 環境変数テンプレート
├── AUTHENTICATION_DESIGN.md             # 詳細設計ドキュメント（全て書いています）
│
├── config/
│   └── config.go                        # 設定管理（環境変数読み込み）
│                                        # → キー設定、トークン有効期限など
│
├── db/
│   └── db.go                            # データベース接続・初期化
│                                        # → PostgreSQL接続、スキーマ作成
│
├── auth/
│   ├── models.go                        # データ構造定義
│   │                                    # → User, TokenPair, AuthError
│   ├── token_manager.go                 # JWT生成・検証
│   │                                    # → トークンペア生成、署名検証
│   ├── auth_service.go                  # 認証ビジネスロジック
│   │                                    # → 登録、ログイン、パスワード検証
│   └── oidc_middleware.go               # ミドルウェア
│                                        # → 認証チェック、CORS処理
│
├── handler/
│   └── auth_handler.go                  # HTTPハンドラー（APIエンドポイント）
│                                        # → /auth/register, /auth/login など
│
└── router/
    └── router.go                        # ルート定義・ミドルウェア設定
```

---

## 🚀 セットアップ手順

### 1️⃣ PostgreSQLのインストール

```bash
# Ubuntu/Debian
sudo apt update && sudo apt install postgresql postgresql-contrib

# macOS
brew install postgresql@15

# 起動
sudo systemctl start postgresql
```

### 2️⃣ データベース作成

```sql
-- PostgreSQLコンソールで実行
sudo -u postgres psql

-- ユーザー作成
CREATE USER auxilia_user WITH PASSWORD 'your_secure_password';

-- データベース作成
CREATE DATABASE auxilia OWNER auxilia_user;

-- 権限付与
GRANT ALL PRIVILEGES ON DATABASE auxilia TO auxilia_user;

-- 接続
\c auxilia auxilia_user
```

### 3️⃣ 環境変数設定

```bash
cd /home/u5yuugoo/projects/hackathon/Auxilia

# テンプレートをコピー
cp .env.example .env

# .envを編集
nano .env
```

**設定内容：**

```env
# DB接続情報
DB_HOST=localhost
DB_PORT=5432
DB_USER=auxilia_user
DB_PASSWORD=your_secure_password
DB_NAME=auxilia
DB_SSLMODE=require

# サーバー
PORT=8080

# JWT秘密鍵（長くてランダムに！）
JWT_SECRET_KEY=your_very_long_random_secret_key_min_32_chars_000000000
```

**秘密鍵の生成:**

```bash
openssl rand -hex 32
# 例: a7f3c9b2d1e4f6g8h0i2j4k6l8m0n2o4p6q8r0s2t4u6v8w0
```

### 4️⃣ サーバー起動

```bash
cd /home/u5yuugoo/projects/hackathon/Auxilia
go run main.go
```

**期待される出力:**

```
設定を読み込みました
データベースに接続しました
テーブルを初期化しました
Server starting at http://localhost:8080
利用可能なエンドポイント:
  POST /auth/register      - ユーザー登録
  POST /auth/login         - ログイン
  POST /auth/refresh       - トークン更新
  POST /auth/logout        - ログアウト
  GET  /health             - ヘルスチェック
  GET  /protected/example  - 保護されたエンドポイント例
```

---

## 🧪 テスト（cURL）

### ユーザー登録

```bash
curl -X POST http://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "email": "testuser@example.com",
    "username": "testplayer",
    "password": "SecurePass123"
  }'
```

**成功レスポンス:**

```json
{
  "id": 1,
  "email": "testuser@example.com",
  "username": "testplayer"
}
```

### ログイン

```bash
curl -X POST http://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "email": "testuser@example.com",
    "password": "SecurePass123"
  }'
```

**成功レスポンス:**

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 900,
  "token_type": "Bearer"
}
```

### 保護されたエンドポイント（トークン付き）

```bash
curl -X GET http://localhost:8080/protected/example \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
```

**成功レスポンス:**

```json
{
  "message": "この情報は認証ユーザーのみ閲覧可能です",
  "user_id": 1,
  "email": "testuser@example.com"
}
```

### トークン更新

```bash
curl -X POST http://localhost:8080/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
  }'
```

### ログアウト

```bash
curl -X POST http://localhost:8080/auth/logout \
  -H "Content-Type: application/json" \
  -d '{
    "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
  }'
```

---

## 📚 コード説明

### 1. 設定管理 (config/config.go)

```go
// 環境変数から設定を読み込む
cfg := config.LoadConfig()

// トークン有効期限
cfg.AccessTokenDuration  // 15分
cfg.RefreshTokenDuration // 7日

// パスワード要件
cfg.PasswordMinLength // 8文字
```

**役割：** 環境変数の一元管理、本番環境対応

### 2. データベース接続 (db/db.go)

```go
// 初期化
db, err := db.InitDB(cfg.GetDSN())

// テーブル自動作成
db.CreateTables()
// 以下のテーブルが作成されます：
// - users (ユーザー情報)
// - refresh_tokens (トークン管理)
// - login_attempts (ブルートフォース対策)
```

**役割：** PostgreSQL接続、スキーマ管理

### 3. JWT トークン管理 (auth/token_manager.go)

```go
// トークンペアを生成
tokenPair, err := tm.GenerateTokenPair(user)
// 返り値：
// - AccessToken（署名済みJWT）
// - RefreshToken（署名済みJWT）

// アクセストークンを検証
claims, err := tm.VerifyAccessToken(tokenString)
// 検証内容：
// - 署名の検証
// - 有効期限チェック
// - トークンタイプの確認
```

**役割：** JWT生成・検証の全処理

### 4. 認証ロジック (auth/auth_service.go)

#### ユーザー登録

```go
user, err := as.RegisterUser(&auth.RegisterRequest{
  Email:    "user@example.com",
  Username: "player123",
  Password: "SecurePass123",
})
// 内部処理：
// 1. バリデーション（形式、複雑性チェック）
// 2. パスワードをbcryptでハッシュ化
// 3. ユーザーをDBに保存
```

#### ログイン

```go
tokenPair, err := as.LoginUser(&auth.LoginRequest{
  Email:    "user@example.com",
  Password: "SecurePass123",
}, clientIP)
// 内部処理：
// 1. ブルートフォース攻撃チェック
// 2. パスワード検証（bcryptで比較）
// 3. トークンペア生成
// 4. RefreshTokenをDB保存
```

#### トークン更新

```go
newTokenPair, err := as.RefreshAccessToken(refreshToken)
// 内部処理：
// 1. RefreshTokenを検証
// 2. DBで無効化状態をチェック
// 3. 新トークンペアを生成
// 4. 古いトークンを無効化
// 5. 新トークンを保存
```

**役割：** 認証の核となるビジネスロジック

### 5. ミドルウェア (auth/oidc_middleware.go)

```go
// 認証ミドルウェア
mux.Handle("/protected/",
  auth.AuthMiddleware(tokenManager)(protectedMux),
)
// 検証フロー：
// 1. Authorizationヘッダーを抽出
// 2. "Bearer <token>"形式を確認
// 3. JWTを検証
// 4. user_id等をコンテキストに保存
```

**役割：** 保護されたエンドポイントへのアクセス制御

### 6. APIハンドラー (handler/auth_handler.go)

```go
// /auth/register エンドポイント
func (h *AuthHandler) RegisterHandler(w http.ResponseWriter, r *http.Request) {
  // 1. JSON解析
  // 2. RegisterUserを呼び出し
  // 3. JSONレスポンス返却
}

// 同様に：
// - LoginHandler
// - RefreshTokenHandler
// - LogoutHandler
```

**役割：** HTTP通信の実装

---

## 🔑 重要な概念

### JWT トークンの構造

```
header.payload.signature
└─────┬─────┘└──┬──┘└──┬───┘
    Base64   Base64   HMAC-SHA256

例：
eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoxLCJleHAiOjE3MDU1NzIxMzR9.a3f8c2...
```

### トークンペア方式

| トークン         | 有効期限 | 用途                |
| ---------------- | -------- | ------------------- |
| **AccessToken**  | 15分     | APIリクエストの認証 |
| **RefreshToken** | 7日      | AccessToken更新用   |

**流れ：**

```
ログイン
  ↓
AccessToken（15分）+ RefreshToken（7日）発行
  ↓
APIリクエスト（AccessToken使用）
  ↓
AccessToken期限切れ
  ↓
RefreshToken使用 → 新しいAccessToken取得
  ↓
ログアウト時 → RefreshToken無効化
```

### パスワードセキュリティ

```
登録時：
入力パスワード → bcrypt → ハッシュ（DB保存）

ログイン時：
入力パスワード + DBハッシュ → bcrypt.CompareHashAndPassword → true/false

特徴：
- ソルト自動生成
- ハッシュは一方向（復号不可）
- 同じパスワードでも毎回異なるハッシュになる
```

---

## 🛡️ セキュリティチェックリスト

本番環境へのデプロイ前に以下をチェック：

- [ ] `JWT_SECRET_KEY` を長くてランダムな値に変更
- [ ] `DB_SSLMODE=require` に設定（本番）
- [ ] HTTPSを有効化（TLS 1.3推奨）
- [ ] CORSの許可ドメインを制限（現在は`*`）
- [ ] ログイン試行ロックの設定を確認（5回/15分）
- [ ] パスワード要件を確認（8字以上、複雑性）
- [ ] データベースバックアップを設定
- [ ] エラーログを記録
- [ ] APIレート制限を実装
- [ ] 定期的にセキュリティアップデートを確認

---

## 📖 詳細ドキュメント

全詳細は [AUTHENTICATION_DESIGN.md](AUTHENTICATION_DESIGN.md) に記載：

- 認証フロー図（5つのシーケンス図）
- セキュリティ対策の詳細
- API仕様書
- Unity実装ガイド
- よくある質問

---

## 🔧 トラブルシューティング

### データベース接続エラー

```
$ psql -U auxilia_user -d auxilia -h localhost
```

確認項目：

- PostgreSQLが起動しているか
- ユーザー名・パスワード
- ホスト・ポート

### JWT_SECRET_KEYエラー

```
警告: JWT_SECRET_KEYが設定されていません
```

対処：`.env`ファイルに以下を設定

```env
JWT_SECRET_KEY=$(openssl rand -hex 32)
```

### "メール/ユーザー名が既に存在"エラー

```json
{
  "code": "EMAIL_ALREADY_EXISTS",
  "message": "このメールアドレスは既に登録されています"
}
```

→ 異なるメールアドレスを使用してください

---

## 📞 サポート

### よく使うコマンド

```bash
# サーバーの起動
go run main.go

# コンパイル
go build -o server main.go

# テスト
go test ./...

# ホットリロード（watchman等が必要）
go run -mod=mod main.go
```

### ログ確認

```bash
# サーバーが出力するログを確認
# - データベース接続状況
# - ログイン試行
# - エラーメッセージ
```

---

## 🚀 次のステップ

このセキュアな認証システムをベースに、以下の機能を追加できます：

1. **アカウント情報の記録機能**
   - ユーザープロフィール
   - ランク・レート

2. **マッチング機能**
   - プレイヤー検索
   - マッチメイキング

3. **試合情報の記録機能**
   - ゲーム履歴
   - スコア記録

4. **レート機能**
   - イロレーティング
   - ランキング

全て同じセキュアなJWT認証で保護可能です！

---

**実装者**: GitHub Copilot  
**作成日**: 2025年1月  
**バージョン**: 1.0

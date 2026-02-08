# Auxilia - セキュアな認証認可システム設計

## 目次

1. [アーキテクチャ概要](#アーキテクチャ概要)
2. [認証フロー](#認証フロー)
3. [セキュリティ対策](#セキュリティ対策)
4. [API仕様](#api仕様)
5. [実装詳細](#実装詳細)
6. [クライアント実装ガイド（Unity）](#クライアント実装ガイドunity)

---

## アーキテクチャ概要

### なぜOIDCではなくJWT + リフレッシュトークン？

| 項目               | OIDC                   | JWT + リフレッシュトークン |
| ------------------ | ---------------------- | -------------------------- |
| **IDプロバイダー** | 外部（Google等）に委譲 | 自前実装                   |
| **ゲーム内管理**   | 困難                   | 容易                       |
| **実装複雑度**     | 高い                   | 中程度                     |
| **ソシャゲ向き**   | 不向き                 | 最適                       |

**採用理由：** ゲーム内でユーザーアカウントを作成・管理する必要があるため

### 全体構成図

```
┌─────────────────────────────────────────────────────────────┐
│                    Unity クライアント                        │
└──────────────────────────────────────────────────────────────┘
                            ↓↑
                    HTTPS (TLS 1.3)
                            ↓↑
┌──────────────────────────────────────────────────────────────┐
│                   Go バックエンド                            │
├──────────────────────────────────────────────────────────────┤
│  ┌────────────────────────────────────────────────────────┐  │
│  │          ルーター (router.go)                          │  │
│  │   ┌─────────────────────────────────────────────┐     │  │
│  │   │      ミドルウェア (CORS, ロギング)         │     │  │
│  │   │  ┌───────────────────────────────────────┐ │     │  │
│  │   │  │  認証ミドルウェア (AuthMiddleware)    │ │     │  │
│  │   │  │   JWT検証・クレーム抽出              │ │     │  │
│  │   │  └───────────────────────────────────────┘ │     │  │
│  │   └─────────────────────────────────────────────┘     │  │
│  │        ↓                                               │  │
│  │  ┌─────────────────────────────────────────────┐     │  │
│  │  │    ハンドラー (handler/auth_handler.go)    │     │  │
│  │  │  ・RegisterHandler (登録)                  │     │  │
│  │  │  ・LoginHandler (ログイン)                │     │  │
│  │  │  ・RefreshTokenHandler (トークン更新)    │     │  │
│  │  │  ・LogoutHandler (ログアウト)            │     │  │
│  │  └─────────────────────────────────────────────┘     │  │
│  │        ↓                                               │  │
│  │  ┌─────────────────────────────────────────────┐     │  │
│  │  │   サービス (auth/auth_service.go)          │     │  │
│  │  │  ・RegisterUser (ユーザー登録)           │     │  │
│  │  │  ・LoginUser (ログイン処理)              │     │  │
│  │  │  ・RefreshAccessToken (トークン更新)    │     │  │
│  │  │  ・RevokeRefreshToken (ログアウト)      │     │  │
│  │  │  ・validateRegistration (バリデーション)│     │  │
│  │  │  ・checkLoginAttempts (ブルートフォース対策)   │     │  │
│  │  └─────────────────────────────────────────────┘     │  │
│  │        ↓                                               │  │
│  │  ┌─────────────────────────────────────────────┐     │  │
│  │  │   トークンマネージャー                      │     │  │
│  │  │   (auth/token_manager.go)                 │     │  │
│  │  │  ・GenerateTokenPair (トークン生成)      │     │  │
│  │  │  ・VerifyAccessToken (アクセス検証)      │     │  │
│  │  │  ・VerifyRefreshToken (リフレッシュ検証) │     │  │
│  │  └─────────────────────────────────────────────┘     │  │
│  └────────────────────────────────────────────────────────┘  │
│                            ↓↑                                 │
│  ┌────────────────────────────────────────────────────────┐  │
│  │           PostgreSQL データベース                      │  │
│  │  ・users (ユーザー情報)                               │  │
│  │  ・refresh_tokens (トークン管理)                     │  │
│  │  ・login_attempts (ブルートフォース対策)             │  │
│  └────────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

---

## 認証フロー

### 1. ユーザー登録フロー

```
クライアント                           サーバー
    │                                    │
    ├─ POST /auth/register ──────────────>│
    │  {                                  │
    │    "email": "user@example.com",     │
    │    "username": "player123",         │
    │    "password": "SecurePass123"      │
    │  }                                  │
    │                                    │
    │  [サーバー処理]                    │
    │  1. バリデーション                │
    │     ・メール形式チェック           │
    │     ・ユーザー名チェック（3-30文字、英数字）
    │     ・パスワード強度チェック       │
    │       - 8字以上                    │
    │       - 大文字、小文字、数字を含む │
    │  2. メール・ユーザー名の重複確認   │
    │  3. bcryptでパスワードをハッシュ化 │
    │  4. ユーザー情報をDBに保存         │
    │                                    │
    │<────────── 201 Created ───────────┤
    │  {                                  │
    │    "id": 1,                         │
    │    "email": "user@example.com",     │
    │    "username": "player123"          │
    │  }                                  │
    │                                    │
```

### 2. ログインフロー

```
クライアント                           サーバー
    │                                    │
    ├─ POST /auth/login ────────────────>│
    │  {                                  │
    │    "email": "user@example.com",     │
    │    "password": "SecurePass123"      │
    │  }                                  │
    │                                    │
    │  [サーバー処理]                    │
    │  1. ブルートフォース攻撃チェック   │
    │     ・過去15分の失敗回数をチェック │
    │     ・5回以上の失敗でロック        │
    │  2. ユーザーをメールで検索         │
    │  3. パスワードをbcryptで検証       │
    │  4. ログイン試行を記録             │
    │  5. JWTトークンペアを生成         │
    │     ・AccessToken: 15分有効       │
    │     ・RefreshToken: 7日有効       │
    │  6. RefreshTokenをハッシュ化して  │
    │     DBに保存（無効化管理用）      │
    │                                    │
    │<────────── 200 OK ────────────────┤
    │  {                                  │
    │    "access_token": "eyJ...",       │
    │    "refresh_token": "eyJ...",      │
    │    "expires_in": 900,              │
    │    "token_type": "Bearer"          │
    │  }                                  │
    │                                    │
```

### 3. API呼び出しフロー（トークン認証）

```
クライアント                           サーバー
    │                                    │
    ├─ GET /protected/example ────────────>│
    │  Headers:                          │
    │  Authorization: Bearer eyJ...      │
    │                                    │
    │  [サーバー処理]                    │
    │  1. AuthorizationヘッダーからToken抽出
    │  2. JWTトークンを検証             │
    │     ・署名チェック                 │
    │     ・有効期限チェック             │
    │     ・TokenTypeが"access"か確認   │
    │  3. クレーム(user_id等)をコンテキストに保存
    │  4. ハンドラーを実行（認証済み）  │
    │                                    │
    │<────── 200 OK ─────────────────────┤
    │  {                                  │
    │    "message": "認証済みです",      │
    │    "user_id": 1,                    │
    │    "email": "user@example.com"      │
    │  }                                  │
    │                                    │
```

### 4. トークン更新フロー

```
クライアント                           サーバー
    │                                    │
    │ [AccessTokenが期限切れに]         │
    │                                    │
    ├─ POST /auth/refresh ──────────────>│
    │  {                                  │
    │    "refresh_token": "eyJ..."       │
    │  }                                  │
    │                                    │
    │  [サーバー処理]                    │
    │  1. RefreshTokenを検証             │
    │  2. TokenTypeが"refresh"か確認   │
    │  3. DBで無効化状態をチェック       │
    │  4. ユーザー情報を再取得          │
    │  5. 新しいTokenPairを生成          │
    │  6. 古いRefreshTokenをDBで無効化  │
    │  7. 新しいRefreshTokenをDB保存    │
    │                                    │
    │<────── 200 OK ─────────────────────┤
    │  {                                  │
    │    "access_token": "eyJ...[新]",  │
    │    "refresh_token": "eyJ...[新]",  │
    │    "expires_in": 900,              │
    │    "token_type": "Bearer"          │
    │  }                                  │
    │                                    │
```

### 5. ログアウトフロー

```
クライアント                           サーバー
    │                                    │
    ├─ POST /auth/logout ───────────────>│
    │  {                                  │
    │    "refresh_token": "eyJ..."       │
    │  }                                  │
    │                                    │
    │  [サーバー処理]                    │
    │  1. RefreshTokenのハッシュを計算   │
    │  2. DBで対応するレコードを無効化   │
    │     UPDATE refresh_tokens          │
    │     SET is_revoked = true          │
    │                                    │
    │<────── 200 OK ─────────────────────┤
    │  {                                  │
    │    "message": "ログアウトしました"  │
    │  }                                  │
    │                                    │
```

---

## セキュリティ対策

### 1. **パスワードセキュリティ**

- ✅ bcrypt（安全な伸展関数）でハッシュ化（ソルト自動生成）
- ✅ パスワード複雑性チェック（大文字+小文字+数字+最小8文字）
- ✅ パスワード平文はDBに保存しない

```
登録時: "SecurePass123"
      ↓ bcrypt (cost=10)
      → "$2a$10$..." (ハッシュ化)

ログイン時: 入力パスワード + DBハッシュ → bcrypt.CompareHashAndPassword()
```

### 2. **JWT トークンセキュリティ**

- ✅ HMAC-SHA256署名（強力な秘密鍵）
- ✅ アクセストークン短期化（15分）
- ✅ リフレッシュトークン長期化（7日）
- ✅ トークンタイプの明示化（"access" / "refresh"を区別）
- ✅ トークンの署名検証（HMAC以外の署名を拒否）

```
アクセストークン内容:
{
  "user_id": 1,
  "email": "user@example.com",
  "username": "player123",
  "token_type": "access",
  "iat": 1705571234,      // 発行時刻
  "exp": 1705572134       // 有効期限（+15分）
}
署名: HMAC-SHA256(header.payload, secret_key)
```

### 3. **ブルートフォース攻撃対策**

- ✅ ログイン試行の記録（email, ip_address, 成功/失敗）
- ✅ N分間にM回以上の失敗でロック（現在: 15分間に5回）
- ✅ クライアントIPで制限（同じIPからの多重攻撃を防止）

```
ログイン試行時:
1. login_attemptsテーブルをチェック
   SELECT COUNT(*)
   FROM login_attempts
   WHERE email = $1
   AND ip_address = $2
   AND success = false
   AND attempted_at > NOW() - INTERVAL '15 minutes'

2. 5回以上なら "TOO_MANY_ATTEMPTS" エラーを返す
3. ログイン成功/失敗を記録
```

### 4. **トークン無効化管理**

- ✅ リフレッシュトークンをハッシュ化してDB保存
- ✅ 生トークンがDB漏洩されても直接利用不可
- ✅ ログアウト時にトークンを無効化
- ✅ 期限切れトークンの自動削除

```
登録時:
token = "eyJ..." (生トークン)
hash = SHA256(token) = "a3f8c2..." (ハッシュ化)
DB保存: hash, user_id, expires_at, is_revoked

ロード時:
入力トークンをハッシュ化 → DBのハッシュと照合
      ↓
is_revoked = false && expires_at > NOW() → 有効
```

### 5. **SQLインジェクション対策**

- ✅ Prepared Statementsを使用（全クエリ）
- ✅ ユーザー入力の直接埋め込みなし

```go
// ✅ 安全（プレースホルダー使用）
db.QueryRow(
    "SELECT id FROM users WHERE email = $1",
    email, // ここに値が安全にバインドされる
)

// ❌ 危険（文字列連結）
db.QueryRow("SELECT id FROM users WHERE email = '" + email + "'")
```

### 6. **データベース接続セキュリティ**

- ✅ SSLモード="require"（本番環境）
- ✅ 接続プール管理（Max 25接続）
- ✅ 接続タイムアウト（5分）

### 7. **CORS対策**

- ✅ CORSミドルウェアを実装
- ⚠️ **本番環境では具体的にUnityクライアントのURLを指定してください**

```go
// 現在（開発環境）
w.Header().Set("Access-Control-Allow-Origin", "*")

// 本番環境ではこのように:
w.Header().Set("Access-Control-Allow-Origin", "https://game.example.com")
```

### 8. **HTTPS/TLS対策**

- ✅ HTTPS(TLS 1.3推奨)を使用
- ✅ 認証ヘッダーは暗号化通信で保護される
- ✅ Cookie送信時はSecure・HttpOnly・SameSiteを設定

### 9. **エラーハンドリング（情報露出防止）**

- ✅ 認証失敗時に詳細を返さない
- ✅ "メールアドレスまたはパスワードが間違っています" （どちらが間違いかを明かさない）

### 10. **レート制限（実装推奨）**

- ⚠️ 現在実装なし（別途実装推奨）
- グローバルなAPIレート制限
- エンドポイント別の制限

---

## API仕様

### 認証エンドポイント

#### POST /auth/register

ユーザー登録

**リクエスト:**

```json
{
  "email": "user@example.com",
  "username": "player123",
  "password": "SecurePass123"
}
```

**レスポンス (201 Created):**

```json
{
  "id": 1,
  "email": "user@example.com",
  "username": "player123"
}
```

**エラーレスポンス (400 Bad Request):**

```json
{
  "code": "EMAIL_ALREADY_EXISTS",
  "message": "このメールアドレスは既に登録されています"
}
```

#### POST /auth/login

ログイン

**リクエスト:**

```json
{
  "email": "user@example.com",
  "password": "SecurePass123"
}
```

**レスポンス (200 OK):**

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 900,
  "token_type": "Bearer"
}
```

#### POST /auth/refresh

トークン更新

**リクエスト:**

```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**レスポンス (200 OK):**

```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIs...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 900,
  "token_type": "Bearer"
}
```

#### POST /auth/logout

ログアウト

**リクエスト:**

```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**レスポンス (200 OK):**

```json
{
  "message": "ログアウトしました"
}
```

### 保護されたエンドポイントの例

#### GET /protected/example

認証が必須なエンドポイント

**リクエスト:**

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

**レスポンス (200 OK):**

```json
{
  "message": "この情報は認証ユーザーのみ閲覧可能です",
  "user_id": 1,
  "email": "user@example.com"
}
```

---

## 実装詳細

### ファイル構造

```
├── main.go                          # エントリーポイント
├── go.mod                           # 依存関係
├── .env.example                     # 環境変数テンプレート
│
├── config/
│   └── config.go                    # 設定管理
│
├── db/
│   └── db.go                        # データベース接続・初期化
│
├── auth/
│   ├── models.go                    # データモデル
│   ├── token_manager.go             # JWT生成・検証
│   ├── auth_service.go              # 認証ビジネスロジック
│   └── oidc_middleware.go           # ミドルウェア
│
├── handler/
│   └── auth_handler.go              # HTTPハンドラー
│
└── router/
    └── router.go                    # ルート定義
```

### 主要なパッケージの役割

#### config.go

- 環境変数からの設定読み込み
- トークン有効期限、パスワード要件の定義
- データベース接続情報の管理

#### db.go

- PostgreSQL接続の初期化
- テーブルスキーマの作成
- コネクションプール管理

#### auth/models.go

- User, TokenPair, JWTClaimsなどのデータ構造

#### auth/token_manager.go

- JWTトークンの生成
- JWTトークンの検証
- 署名方式の確認

#### auth/auth_service.go

- **核となるロジック**
- ユーザー登録（バリデーション + パスワードハッシュ化）
- ログイン（パスワード検証 + ブルートフォース対策）
- トークン更新
- トークン無効化

#### auth/oidc_middleware.go

- Authorizationヘッダーの検証
- トークン検証後のコンテキスト設定
- CORS処理
- ロギング

#### handler/auth_handler.go

- HTTPリクエスト/レスポンス処理
- JSON エンコード/デコード
- エラーハンドリング

#### router/router.go

- ルート定義
- ミドルウェアの適用

---

## クライアント実装ガイド（Unity）

### 1. ユーザー登録

```csharp
public class AuthManager : MonoBehaviour
{
    private string serverUrl = "https://game.example.com";

    public async void Register(string email, string username, string password)
    {
        using (var client = new HttpClient())
        {
            var registerData = new
            {
                email = email,
                username = username,
                password = password
            };

            var json = JsonConvert.SerializeObject(registerData);
            var content = new StringContent(json, Encoding.UTF8, "application/json");

            try
            {
                var response = await client.PostAsync(
                    $"{serverUrl}/auth/register",
                    content
                );

                if (response.IsSuccessStatusCode)
                {
                    string responseBody = await response.Content.ReadAsStringAsync();
                    Debug.Log("登録成功: " + responseBody);
                }
                else
                {
                    string errorBody = await response.Content.ReadAsStringAsync();
                    Debug.LogError("登録失敗: " + errorBody);
                }
            }
            catch (Exception e)
            {
                Debug.LogError("ネットワークエラー: " + e.Message);
            }
        }
    }
}
```

### 2. ログイン

```csharp
public async void Login(string email, string password)
{
    using (var client = new HttpClient())
    {
        var loginData = new
        {
            email = email,
            password = password
        };

        var json = JsonConvert.SerializeObject(loginData);
        var content = new StringContent(json, Encoding.UTF8, "application/json");

        try
        {
            var response = await client.PostAsync(
                $"{serverUrl}/auth/login",
                content
            );

            if (response.IsSuccessStatusCode)
            {
                string responseBody = await response.Content.ReadAsStringAsync();
                var tokenResponse = JsonConvert.DeserializeObject<TokenResponse>(responseBody);

                // トークンをローカルに保存
                PlayerPrefs.SetString("access_token", tokenResponse.AccessToken);
                PlayerPrefs.SetString("refresh_token", tokenResponse.RefreshToken);
                PlayerPrefs.SetInt("expires_in", tokenResponse.ExpiresIn);
                PlayerPrefs.SetString("token_obtained_time", System.DateTime.Now.ToString());

                Debug.Log("ログイン成功");
            }
        }
        catch (Exception e)
        {
            Debug.LogError("ログインエラー: " + e.Message);
        }
    }
}
```

### 3. トークン付きAPIリクエスト

```csharp
public async void GetProtectedData()
{
    var accessToken = PlayerPrefs.GetString("access_token");

    using (var client = new HttpClient())
    {
        var request = new HttpRequestMessage(
            HttpMethod.Get,
            $"{serverUrl}/protected/example"
        );

        // Authorizationヘッダーにトークンを設定
        request.Headers.Add("Authorization", $"Bearer {accessToken}");

        try
        {
            var response = await client.SendAsync(request);

            if (response.IsSuccessStatusCode)
            {
                string responseBody = await response.Content.ReadAsStringAsync();
                Debug.Log("保護されたデータ取得成功: " + responseBody);
            }
            else if (response.StatusCode == System.Net.HttpStatusCode.Unauthorized)
            {
                // トークン期限切れ → 更新
                await RefreshToken();
            }
        }
        catch (Exception e)
        {
            Debug.LogError("API呼び出しエラー: " + e.Message);
        }
    }
}
```

### 4. トークン更新

```csharp
public async Task RefreshToken()
{
    var refreshToken = PlayerPrefs.GetString("refresh_token");

    using (var client = new HttpClient())
    {
        var refreshData = new { refresh_token = refreshToken };
        var json = JsonConvert.SerializeObject(refreshData);
        var content = new StringContent(json, Encoding.UTF8, "application/json");

        try
        {
            var response = await client.PostAsync(
                $"{serverUrl}/auth/refresh",
                content
            );

            if (response.IsSuccessStatusCode)
            {
                string responseBody = await response.Content.ReadAsStringAsync();
                var tokenResponse = JsonConvert.DeserializeObject<TokenResponse>(responseBody);

                // 新しいトークンを保存
                PlayerPrefs.SetString("access_token", tokenResponse.AccessToken);
                PlayerPrefs.SetString("refresh_token", tokenResponse.RefreshToken);

                Debug.Log("トークン更新成功");
            }
        }
        catch (Exception e)
        {
            Debug.LogError("トークン更新エラー: " + e.Message);
        }
    }
}

public class TokenResponse
{
    [JsonProperty("access_token")]
    public string AccessToken { get; set; }

    [JsonProperty("refresh_token")]
    public string RefreshToken { get; set; }

    [JsonProperty("expires_in")]
    public int ExpiresIn { get; set; }

    [JsonProperty("token_type")]
    public string TokenType { get; set; }
}
```

---

## セットアップ手順

### 1. 依存パッケージの取得

```bash
go mod download
```

### 2. 環境変数の設定

```bash
cp .env.example .env
# .envを編集して、DB接続情報とJWT_SECRET_KEYを設定
```

### 3. PostgreSQLのセットアップ

```sql
-- ユーザー作成
CREATE USER auxilia_user WITH PASSWORD 'your_password';
CREATE DATABASE auxilia OWNER auxilia_user;
GRANT ALL PRIVILEGES ON DATABASE auxilia TO auxilia_user;
```

### 4. サーバー起動

```bash
go run main.go
```

### 5. テスト

```bash
# ユーザー登録
curl -X POST https://localhost:8080/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","username":"player123","password":"SecurePass123"}'

# ログイン
curl -X POST https://localhost:8080/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"SecurePass123"}'
```

---

## 今後の拡張

### セキュリティ強化

- [ ] レート制限（API呼び出し回数制限）
- [ ] HTTPS/TLS 1.3の導入
- [ ] パスワードリセット機能
- [ ] メール確認機能
- [ ] 二要素認証（2FA）
- [ ] セッション管理の改善

### 機能追加

- [ ] アカウント情報更新
- [ ] プロフィール画像アップロード
- [ ] ソーシャルログイン（Google等）
- [ ] デバイス管理
- [ ] ログイン履歴

### データベース最適化

- [ ] インデックス追加
- [ ] パーティショニング
- [ ] バックアップ戦略

---

## よくある質問

### Q1: JWTトークンは長く保持しても安全？

**A:** いいえ。AccessTokenは短期（15分）にし、RefreshTokenで更新する設計にしています。

### Q2: RefreshTokenが盗まれたら？

**A:** DBでトークンのハッシュを管理し、無効化できます。また、ログアウト時に自動無効化します。

### Q3: パスワード忘れた場合は？

**A:** 現在実装していません。メール確認＋リセットトークン機能の追加を検討してください。

### Q4: 本番環境で注意することは？

1. JWT_SECRET_KEYを長くてランダムに（例：openssl rand -hex 32）
2. DB_SSLMODEをrequireに設定
3. HTTPSを有効にする
4. CORSの許可ドメインを制限

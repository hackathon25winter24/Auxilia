package main

// このファイルはサンプル実装用です。
// 実際のコードはmain.goで実行されます

import "fmt"

// ===============================================
// Unity クライアント実装の参考コード
// (C#)
// ===============================================

/*

using System;
using System.Collections;
using UnityEngine;
using UnityEngine.Networking;
using Newtonsoft.Json;

public class AuxiliaAuthManager : MonoBehaviour
{
    private const string BASE_URL = "https://game.example.com";
    private string accessToken;
    private string refreshToken;

    // ==========================================
    // 1. ユーザー登録
    // ==========================================
    public IEnumerator Register(string email, string username, string password)
    {
        var registerData = new
        {
            email = email,
            username = username,
            password = password
        };

        var json = JsonConvert.SerializeObject(registerData);
        var www = new UnityWebRequest($"{BASE_URL}/auth/register")
        {
            uploadHandler = new UploadHandlerRaw(System.Text.Encoding.UTF8.GetBytes(json)),
            downloadHandler = new DownloadHandlerBuffer(),
            method = UnityWebRequest.kHttpVerbPOST
        };
        www.SetRequestHeader("Content-Type", "application/json");

        yield return www.SendWebRequest();

        if (www.result == UnityWebRequest.Result.Success)
        {
            Debug.Log("登録成功");
            var response = JsonConvert.DeserializeObject<RegisterResponse>(www.downloadHandler.text);
            Debug.Log($"ユーザーID: {response.id}");
        }
        else
        {
            Debug.LogError($"登録失敗: {www.error}");
        }
    }

    // ==========================================
    // 2. ログイン
    // ==========================================
    public IEnumerator Login(string email, string password)
    {
        var loginData = new
        {
            email = email,
            password = password
        };

        var json = JsonConvert.SerializeObject(loginData);
        var www = new UnityWebRequest($"{BASE_URL}/auth/login")
        {
            uploadHandler = new UploadHandlerRaw(System.Text.Encoding.UTF8.GetBytes(json)),
            downloadHandler = new DownloadHandlerBuffer(),
            method = UnityWebRequest.kHttpVerbPOST
        };
        www.SetRequestHeader("Content-Type", "application/json");

        yield return www.SendWebRequest();

        if (www.result == UnityWebRequest.Result.Success)
        {
            var response = JsonConvert.DeserializeObject<TokenResponse>(www.downloadHandler.text);
            accessToken = response.access_token;
            refreshToken = response.refresh_token;

            // トークンをローカルストレージに保存
            PlayerPrefs.SetString("access_token", accessToken);
            PlayerPrefs.SetString("refresh_token", refreshToken);
            PlayerPrefs.Save();

            Debug.Log("ログイン成功");
        }
        else
        {
            Debug.LogError($"ログイン失敗: {www.error}");
        }
    }

    // ==========================================
    // 3. 保護されたAPIの呼び出し
    // ==========================================
    public IEnumerator GetProtectedData()
    {
        var www = new UnityWebRequest($"{BASE_URL}/protected/example")
        {
            downloadHandler = new DownloadHandlerBuffer(),
            method = UnityWebRequest.kHttpVerbGET
        };

        // Authorizationヘッダーにトークンを設定
        www.SetRequestHeader("Authorization", $"Bearer {accessToken}");

        yield return www.SendWebRequest();

        if (www.result == UnityWebRequest.Result.Success)
        {
            Debug.Log($"データ取得成功: {www.downloadHandler.text}");
        }
        else if (www.responseCode == 401)
        {
            // トークン期限切れ -> 更新
            yield return RefreshAccessToken();
            yield return GetProtectedData(); // リトライ
        }
        else
        {
            Debug.LogError($"エラー: {www.error}");
        }
    }

    // ==========================================
    // 4. トークン更新
    // ==========================================
    public IEnumerator RefreshAccessToken()
    {
        var refreshData = new { refresh_token = refreshToken };
        var json = JsonConvert.SerializeObject(refreshData);

        var www = new UnityWebRequest($"{BASE_URL}/auth/refresh")
        {
            uploadHandler = new UploadHandlerRaw(System.Text.Encoding.UTF8.GetBytes(json)),
            downloadHandler = new DownloadHandlerBuffer(),
            method = UnityWebRequest.kHttpVerbPOST
        };
        www.SetRequestHeader("Content-Type", "application/json");

        yield return www.SendWebRequest();

        if (www.result == UnityWebRequest.Result.Success)
        {
            var response = JsonConvert.DeserializeObject<TokenResponse>(www.downloadHandler.text);
            accessToken = response.access_token;
            refreshToken = response.refresh_token;

            PlayerPrefs.SetString("access_token", accessToken);
            PlayerPrefs.SetString("refresh_token", refreshToken);
            PlayerPrefs.Save();

            Debug.Log("トークン更新成功");
        }
        else
        {
            // リフレッシュトークンも期限切れ -> 再ログイン必要
            Debug.LogError("再度ログインしてください");
        }
    }

    // ==========================================
    // 5. ログアウト
    // ==========================================
    public IEnumerator Logout()
    {
        var logoutData = new { refresh_token = refreshToken };
        var json = JsonConvert.SerializeObject(logoutData);

        var www = new UnityWebRequest($"{BASE_URL}/auth/logout")
        {
            uploadHandler = new UploadHandlerRaw(System.Text.Encoding.UTF8.GetBytes(json)),
            downloadHandler = new DownloadHandlerBuffer(),
            method = UnityWebRequest.kHttpVerbPOST
        };
        www.SetRequestHeader("Content-Type", "application/json");

        yield return www.SendWebRequest();

        if (www.result == UnityWebRequest.Result.Success)
        {
            PlayerPrefs.DeleteKey("access_token");
            PlayerPrefs.DeleteKey("refresh_token");
            PlayerPrefs.Save();

            accessToken = null;
            refreshToken = null;
            Debug.Log("ログアウト完了");
        }
        else
        {
            Debug.LogError($"ログアウト失敗: {www.error}");
        }
    }
}

// ==========================================
// レスポンスモデル
// ==========================================

[System.Serializable]
public class RegisterResponse
{
    public int id;
    public string email;
    public string username;
}

[System.Serializable]
public class TokenResponse
{
    public string access_token;
    public string refresh_token;
    public int expires_in;
    public string token_type;
}

*/

// ===============================================
// Go サーバー側の使用例
// ===============================================

/*

package main

import (
    "auxilia/auth"
    "auxilia/config"
)

func main() {
    cfg := config.LoadConfig()
    tm := auth.NewTokenManager(
        cfg.JWTSecretKey,
        cfg.AccessTokenDuration,
        cfg.RefreshTokenDuration,
    )
    as := auth.NewAuthService(cfg, tm)

    // 1. ユーザー登録
    user, err := as.RegisterUser(&auth.RegisterRequest{
        Email:    "player@example.com",
        Username: "gamer123",
        Password: "SecurePass123",
    })
    if err != nil {
        panic(err)
    }
    fmt.Printf("登録成功: %+v\n", user)

    // 2. ログイン
    tokenPair, err := as.LoginUser(&auth.LoginRequest{
        Email:    "player@example.com",
        Password: "SecurePass123",
    }, "192.168.1.1")
    if err != nil {
        panic(err)
    }
    fmt.Printf("トークン: %s\n", tokenPair.AccessToken)

    // 3. トークン検証
    claims, err := tm.VerifyAccessToken(tokenPair.AccessToken)
    if err != nil {
        panic(err)
    }
    fmt.Printf("ユーザーID: %d\n", claims.UserID)

    // 4. ログアウト
    err = as.RevokeRefreshToken(tokenPair.RefreshToken)
    if err != nil {
        panic(err)
    }
    fmt.Println("ログアウト完了")
}

*/

func exampleCode() {
	fmt.Println("詳細な実装例は上記のコメントを参照")
}

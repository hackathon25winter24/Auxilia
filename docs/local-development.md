# Unity開発用ローカルサーバー

## 起動

Docker DesktopをLinuxコンテナモードで起動し、Auxiliaリポジトリのルートで実行する。

```powershell
docker compose -f compose.local.yml up -d --build --wait --wait-timeout 180
```

この一コマンドでイメージをビルドし、MariaDBの準備完了を待ち、Auxiliaを起動する。
AuxiliaはDBのマイグレーション後、認証・ユーザー・Room/Lobby・戦闘V2・時間経過処理を同じプロセスで提供する。
`--wait`は両サービスのhealthcheck成功を待つ。初回はイメージ取得とGo依存のダウンロードに時間がかかる。
Docker Desktopが停止している場合は先に起動する。Windowsでは`docker desktop start`も使用できる。

workspaceルートからなら、`-f Unity/Auxilia/compose.local.yml`と指定する。
既存の`docker-compose.yml`ではなく、必ず`compose.local.yml`を使用する。

接続先：`http://localhost:18080/`。HTTPのgRPC-Webを使用する。TLS/Caddyは不要。
ポート公開は127.0.0.1のみ。他のPC・スマートフォンからの接続はこの構成の対象外。
DBはホストへ公開しない。Composeプロジェクト名は`auxilia-local`。

## Unity

Unity Editorでは接続先の既定値を`http://localhost:18080/`にしている。
Play Modeを開始するとNetworkClientCoreが実際の接続先をConsoleへ出力する。

- ローカル：`Tools > Auxilia > Server > Local (localhost 18080)`
- 本番：`Tools > Auxilia > Server > Production`

変更後はPlay Modeを終了して再開する。Scene・ゲームUI・アセットの変更は不要。
Editorの選択はEditorPrefsに保存され、次回も継続する。ビルドしたプレイヤーの既定値は本番URLのまま。
環境変数`AUXILIA_SERVER_URL`があればEditor設定・プレイヤー既定値より優先する。
環境変数を変更した場合はUnityを起動し直し、Unityプロセスへ引き継がせる。
HTTP(S)の絶対URLのみ許可し、URL中のユーザー名・パスワード・クエリ・フラグメントは拒否する。

例：ローカルポートを18082へ変更する場合。

```powershell
$env:AUXILIA_LOCAL_PORT = "18082"
docker compose -f compose.local.yml up -d --build --wait --wait-timeout 180
$env:AUXILIA_SERVER_URL = "http://localhost:18082/"
# この環境変数を継承するプロセスとしてUnityを起動する。
```

アプリ内部の待受は8080固定。AUXILIA_LOCAL_PORTはホスト側ポートだけを変更する。
本番URLへ自動フォールバックしない。ローカル停止時はローカルへの通信が失敗する。

## 状態・ログ・停止

```powershell
docker compose -f compose.local.yml ps
curl.exe http://localhost:18080/healthz
docker compose -f compose.local.yml logs -f app
docker compose -f compose.local.yml logs -f db
docker compose -f compose.local.yml down
```

`/healthz`は正常時HTTP 200と`{"status":"ok"}`を返す。
マイグレーション失敗では待受開始前に終了する。起動後DBに接続できなくなった場合はHTTP 503となる。
`/healthz`成功は接続・起動確認であり、全ゲーム操作の正しさを保証するテストではない。
gRPC-Web APIを確認するときはProtoに従ってリクエストする。ブラウザでルートURLを開いた結果だけで稼働判定しない。

`down`はコンテナとネットワークを停止・削除し、DBボリュームを残す。
次回起動時にアカウントや試合データが継続する。DBには本番データを取り込まず、ローカル用アカウントを作る。
`down -v`はDBボリュームも削除するため、全ローカルデータを破棄すると決めた場合にだけ使用する。
自動再起動設定は付けていない。起動失敗・クラッシュ時はログと終了コードを確認する。

## 開発・更新

バックエンドの変更後は起動コマンドを再実行してリビルドする。ソースコードのホットリロードは行わない。
通常の停止・起動は冪等で、既存の専用DBボリュームを再利用する。
DB初期化用のユーザー名・パスワードはローカル専用の固定値。稼働済みボリュームに対してComposeの値だけ変えてもDBユーザーは再作成されない。
本番用の.envはイメージに含めず、ローカルDB接続先はComposeで明示している。

既存UIの新UseCaseへの接続状況は変更していない。本設定はバックエンドを起動して接続できるようにするもので、
全画面の完成、テストデータ投入、試合自動作成、全操作の自動実行は行わない。

## トラブルシューティング

| 症状 | 確認 |
|---|---|
| Docker APIへ接続できない | Docker Desktopの起動、Linuxコンテナモード |
| port is already allocated | 18080の使用状況。必要ならAUXILIA_LOCAL_PORTを変更 |
| db unhealthy | dbログ、初期化に必要な時間、ディスク空き容量 |
| app exited / unhealthy | appログ。DB接続、migration failed、Server listeningを確認 |
| Unityが本番へ接続する | ConsoleのURL、Editorメニュー、AUXILIA_SERVER_URLを確認 |
| 接続先変更が反映されない | Play Mode再開始。環境変数の場合はUnityプロセスを再起動 |
| 登録成功後に別処理で失敗 | SignUpは登録とログインをまとめているため、既に作成済みか確認 |

## 検証記録（2026-10-22）

- Docker Composeで実際にビルド・起動し、db/appがhealthyになることを確認。
- ホストからhealthzのHTTP 200を確認。
- gRPC-WebでCreateUserへ空入力を送り、HTTP 200＋grpc-status=3（入力エラー）を確認。
- GetDefinitionsとGetLobbyへの未認証要求はHTTP 200＋grpc-status=16。HTMLの503ではなく、各RPCが応答していることを確認。
- `go test ./...`は成功。実アカウントの作成や全ゲーム操作は今回検証していない。
- Unityの接続先テストを追加。プロジェクトが既存Editorで使用中のため、別Editorによる実行は未実施。
  開いているEditorで`Tools > Auxilia > Validate server configuration`を実行可能。

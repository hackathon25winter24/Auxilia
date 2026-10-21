# LobbyServiceV2

部屋の状態をサーバーが決定する追加API。既存RoomServiceV2／RoomMatchServiceV2は残す。
正本は`proto/v2/lobbyV2.proto`。
全体計画・API仕様・状態遷移・フロント側の後続作業は、UIリポジトリの
`docs/room-authoritative-v2-plan.md`に記録する。

## 実装

- `infrastructure/gormv2/lobbyV2.go`：部屋行ロック、操作、全Snapshot、永続revision。
- `handler/grpcv2/lobbyV2.go`：Bearer認証、RPC、500ms間隔の変更配信。
- `lobby_v2_versions`：部屋ID、revision、公開状態ハッシュ。起動時にAutoMigrate。
- `Store.Create`：全ての試合作成経路で所有者かつ部屋メンバーであることを要求。

WatchRoomはサーバーストリーム。クライアントは切断後の再接続で初回全Snapshotを取得し、
room_idとrevisionで古い応答を無視する。購読解除は退出操作ではない。
Snapshotは状態同期用であり、全変更のイベント履歴ではない。
gRPC-Webで利用でき、既存の双方向StreamRoomを使用する必要はない。

## 互換性と運用

既存RoomServiceV2.StartMatch／BattleServiceV2.CreateGameも所有者以外はPermissionDeniedになる。
旧JoinRoomの再入室は席・準備状態を保持する。旧UpdateRoomStateの観戦者ready=trueは拒否する。
旧APIは互換経路として残るため、新UIはLobbyServiceV2の操作APIへ切り替える。
更新系RPCは結果不明時に再取得する。RoomCommandのexpected_revision不一致はAborted。
CreateLobbyは自動再送しない（作成の冪等キーは未導入）。

joined_atを最終通信時刻として扱う旧自動削除を停止した。自動退出のleaseは未導入。
切断ユーザーと放置部屋は保持される。待機中の明示退出時に所有者移譲・空室削除を行う。
対戦中の切断・敗北判定は既存戦闘の規則に委ねる。

## 検証

`go test ./...`、`go vet ./handler/grpcv2 ./infrastructure/gormv2`。
テストは認証、メンバー権限、旧経路の開始権限、競合、席・定員、準備解除、
所有者移譲、編成・戦闘・終了後復帰、gRPC-Web初回/更新配信、旧APIの変更検出を対象とする。
`AUXILIA_V2_TEST_MYSQL_DSN`が設定されていればStoreのテストは専用一時DBでMySQLも検証する。
未設定時はSQLite。実環境のMySQL・Unityクライアント・デプロイ検証を代替するものではない。

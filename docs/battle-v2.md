# 戦闘サーバー V2

要求・応答フィールド、呼び出し条件、エラーの一覧は [V2 APIリファレンス](battle-v2-api.md) を参照。

## 実装と移植元

Auxilia-webserver の `internal/game` を `domain/gamev2` に移植した。
移植元コミット: `8ad95745c5b01cb651a63fd205d7e3fe1e441f80`。
ルール識別子: `web-2026-09-26-v2`。

移動、攻撃範囲・方向、HP、コスト、地形、状態異常、キャラクター固有能力、
復活、ターン終了・開始処理、勝敗はすべてサーバーが決定する。
拠点HPは400、最大コスト50、操作時間120秒、終了処理フェーズ2秒。
16キャラクターの定義、先手決定、配置順もWeb版に従う。
Web版のテストモード関数もエンジンには残すが、通常対戦APIには公開しない。
Web版のゲスト管理・使用率集計・HTTP APIは移植対象外。

元の `game.go`、旧proto、旧生成コード、旧DBモデルは削除していない。
起動時に旧BattleService・RoomService・RoomMatchServiceは登録しない。
旧戦闘・旧ロビーのRPCは `UNIMPLEMENTED` になる。旧DBの戦闘データも変更しない。
`UserService` は継続し、UpdateUser/DeleteUserだけV2認証ガードを通す。
UpdateUserからのレート・勝利数・対戦数は無視し、最新DB値を維持する。
DeleteUserはルームから退出済みであることが必要。

## 構成

- `domain/gamev2/`: 通信・DBに依存しない戦闘エンジンと回帰テスト。
- `infrastructure/gormv2/`: 試合、セッション、トランザクション、結果確定。
- `handler/grpcv2/`: 操作RPC、状態配信、V2ロビー、期限監視。
- `proto/v2/`: `gameV2.proto`、`roomV2.proto`、`roomMatchV2.proto`。
- `pb/v2/`: コミット済みGo生成コード。

通信は既存と同じポートでgRPC／gRPC-Webを受け付ける。
戦闘はUnary RPC送信とserver streaming受信、ルームは従来同様bidirectional streaming。
gRPC-Web WebSocket要求はWebSocketを有効化したラッパーへルーティングする。
実際のUnityプラットフォームでの双方向通信は既存の対応トランスポートを使用すること。

## クライアントの接続順

1. 必要なら既存 `user.UserService/CreateUser` でアカウントを作成する。
2. `game.network.v2.BattleServiceV2/Login` に `name` と `password` を送る。
   戻り値の `token`、`player_id`、`expires_at` を保持する。
3. 以降、V2の全RPCと既存UpdateUser/DeleteUserに
   `authorization: Bearer <token>` をmetadataとして付ける。ストリーム開始時にも必要。
   セッション期限は24時間。期限切れでは再ログインしてストリームを張り直す。
   DBにはトークンのSHA-256ハッシュだけを保存する。
4. `RoomMatchServiceV2/CreateRoomMatch` に自身の `owner_id` と部屋名を送る。
   作成者は自動で観戦枠に入室する。対戦相手は `RoomServiceV2/JoinRoom` で入室する。
5. `RoomServiceV2/EnterRing` または `UpdateRoomState` で1P/2P枠に入る。
   `SetReady` で両者が準備完了にする。ユーザーIDは認証した本人と一致する必要がある。
6. どちらかが `BattleServiceV2/CreateGame { room_id }` を呼ぶ。
   ルームの両枠・準備状態を検査し、V2試合作成と `is_gaming=true` を同一トランザクションで行う。
   `RoomServiceV2/StartMatch` でも同じ試合作成を行える。この戻り値の `started` は試合作成を表す。
   戦闘の実開始は後述の両者Ready後。
7. `GetRoomGame { room_id }` で試合IDを取得できる。既存の未終了試合を優先して返す。
   `CreateGame` の再送でも同じ未終了試合を返す。
8. 両者が `RegisterCharacters` に3体の文字列 `definition_ids` を送る。
   `GetDefinitions` の定義IDを使用する。重複・未定義IDは拒否する。
9. 両者が `Ready { match_id }` を送ると戦闘開始。
   編成変更は開始前だけ可能で、変更時は両者のReadyを解除する。
   準備を中止するときは `CancelGame`。開始後は `Surrender` を使う。
10. `StreamGame { match_id }` を購読し、移動・技・ターン終了などをUnary RPCで送る。
    `GetGameData` は再接続・同期復旧用。接続時は最新の全状態を配信する。
    ルーム内の観戦者も状態・履歴を取得できるが、操作は参加者2名だけに限定する。
11. `finished=true` の状態で結果を表示する。終了時にルームの対戦中フラグとReadyを解除する。
    再戦は再度ルームReady→CreateGameを行い、別の `match_id` を使用する。
    終了済み試合の状態・履歴は試合IDで引き続き取得できる。

旧 `SetReady` の自動試合開始や、クライアントからの `NewTurn` は使わない。
対戦準備中・対戦中はルーム枠や所属を変更できないため、CancelGameまたは降参で終了させてから退出する。

## 操作形式

`ApplyMove`、`ApplyAttack`、`EndTurn`、`Surrender` は共通の `ActionRequest` を使用する。

```json
{
  "match_id": "<CreateGameで得たUUID>",
  "command_id": "<操作ごとのUUID>",
  "expected_revision": 12,
  "character_id": "p1-c1",
  "attack_index": 0,
  "target": { "x": 3, "y": 2 },
  "direction": { "x": 1, "y": 0 }
}
```

- `expected_revision`: 最後に受信した `state.revision`。固定値ではない。
- `character_id`: 試合内ID。定義IDや旧数値マスターIDと区別する。
- `attack_index`: 0始まり。形態変化後の技もサーバーが選ぶ。
- `target`: 移動・技で必須。自分対象の技では自分の座標を指定する。
- `direction`: 技では必須。右(1,0)、左(-1,0)、上(0,1)、下(0,-1)のいずれか。
  Unity画面の上下は描画座標系に合わせて変換する。
- 同一操作の通信再送は同じ `command_id`・同じ内容を送る。
  処理済み操作なら再適用せず現在の全状態を返す（最初の応答のコピーではない）。
  同じIDを異なる内容に使うと `ALREADY_EXISTS`。
- `ABORTED` はrevision不一致。状態を再取得して操作を再評価し、別IDで送る。
  古い操作をそのまま無条件で再実行しない。
- `INVALID_ARGUMENT` は不正な移動・技・入力、`FAILED_PRECONDITION` は手番・開始条件違反。
  `UNAUTHENTICATED` は認証不備、`PERMISSION_DENIED` は他人の操作や非参加者の操作。

HP、地形、効果などの計算結果を送るAPIは存在しない。
コスト・ダメージ等の表示用予測を行う場合でも、サーバー状態で最終確定する。

## 演出と同期

Snapshotは全戦闘状態、登録済み編成のプレイヤーID、レート結果、`last_log_sequence`を返す。
特殊状態（使用済み技・復活・形態・二日酔い等）もprotobufに含む。
キャラクター定義一覧は `definitions_json`。Web版CharacterDefinitionの構造で返す。
旧数値キャラクターIDと文字列IDの対応はUnityの素材・定義に合わせて設定する。

各状態変更は専用テーブルに構造化して保存する。
`FetchActionLog { match_id, after_sequence, limit }` は操作、実行者、操作前・操作後の全状態を返す。
通常はSnapshotに同梱する構造化演出イベントを使い、最終状態へ同期する。
追撃・ノックバック・地形ダメージ・復活等を個別に記録する。
直近32遷移を同梱し、それより古い欠落分だけ履歴から補う。
追加フィールドとUnity側の手順は [演出イベント仕様](battle-v2-presentation.md) を参照。
`limit` は既定・最大100。`next_sequence` を次の `after_sequence` に使ってページ送りする。
取得件数が0になるまで追従できる。ログはWeb版の直近30件制限とは別に永続化する。

`state.revision` と `last_log_sequence` は別の番号。
1操作が複数のエンジンイベントを発生させるため、revisionは連続するとは限らない。
ストリームとUnary応答が前後して届く場合、古い `last_log_sequence` のSnapshotで上書きしない。
配信の間に複数操作が進んでも、ログから欠落した演出・状態遷移を取得できる。

## 保存と期限処理

起動時に次のテーブルを追加する。既存の戦闘テーブルは利用しない。

- `battle_v2_matches`: ルーム紐付け、JSON状態・編成、結果・レート。
- `battle_v2_commands`: 試合・本人・コマンドIDごとの処理済み記録。
- `battle_v2_transitions`: 連番付きの状態遷移履歴。
- `battle_v2_sessions`: 有効期限付きセッション。

ルーム→試合→結果確定時のユーザーID順にロックする。
操作・履歴・処理済み記録・勝敗／レート更新は同じトランザクションで確定する。
InnoDBのREPEATABLE READで古いコマンド記録を読まないよう、試合のルームID確認は
更新トランザクションの開始前に行い、ロック取得後の状態と記録を使用する。

250msごとのワーカーが保存済み期限を監視し、操作がなくてもターンを進める。
再起動時もDBから未終了試合を探索する。期限切れに伴う更新は、受信操作が古くて
拒否された場合にも保存する。長時間停止後の全ターンを早送りすることはせず、
Web版と同様、期限切れ→終了処理→次手番の順に復旧する。

ストリームは250msごとにDBの遷移番号を確認し、変化時だけ送信する。
同一ストリームへのSendは1つのgoroutineだけが行う。別プロセスによる更新も通知する。
複数レプリカの期限処理もDBロックで直列化する。
保存済みのV2履歴・コマンドは自動削除しない。長期運用時は容量と保持期間を別途設計する。

## テスト・生成

```sh
go test ./...
go vet ./...
```

通常のDBテストはテスト専用SQLite（Pure Go）を使う。本番DBドライバは従来どおりMySQL。
実MySQL/MariaDBで同じ保存テストを実行する場合、専用テストサーバーのDSNを
`AUXILIA_V2_TEST_MYSQL_DSN` に設定して `go test ./infrastructure/gormv2 -count=1` を実行する。
各テストは `battle_v2_test_<UUID>` という新しいDBだけを作成・削除するため、
このテスト用ユーザーにCREATE/DROP DATABASE権限が必要。本番DSNは指定しない。

保存テストには同時作成、同じ操作の同時再送、期限切れと拒否操作、再起動相当の復元、
結果の一度だけの反映、失敗時のロールバック、再戦時の履歴保持を含む。
通信テストは実gRPCのUnary／StreamとHTTP経由gRPC-WebのUnary／server streamを通す。
旧API未登録とプロフィールAPIからのレート上書き防止も検査する。

移植元の `internal/game/test` は別ディレクトリのため未コンパイルだった。
V2では同じパッケージへ配置し、現行仕様と衝突した期待値を修正した。
対象: 16体への増加、睡魔の威力30・回復50、カスイマの威力30/10と2手番の効果期間、
ダーナ直接攻撃の毒100%、ナディアの確定半威力追撃。
Web版の現行仕様書と既存の仕様更新テストを基準とし、移植元ファイルは変更していない。
エンジンには使用済み技エラーの分類と演出イベントの観測点を追加している。
戦闘ルールの計算結果・revision・乱数の入力は変更しない。

Goコードの再生成にはprotoc 29.3、protoc-gen-go v1.36.11、protoc-gen-go-grpc v1.5.1を使用した。
各pluginをPATHに配置して、Auxiliaルートから次を実行する。

```sh
protoc -I proto --go_out=. --go_opt=module=auxilia --go-grpc_out=. --go-grpc_opt=module=auxilia proto/v2/gameV2.proto proto/v2/roomV2.proto proto/v2/roomMatchV2.proto
```

UnityではV2の3つのprotoと、import元の `room.proto`、`room_match.proto` から
使用中のC# gRPC生成手順でクライアントを生成する。V2のC#名前空間は `Game.Network.V2`。
ルーム系メッセージは元protoの名前空間を使用する。

## 導入時の注意

サーバーとUnityクライアントを同時にV2へ切り替えること。
旧クライアントは旧戦闘・旧ロビーAPIを呼ぶため、そのまま接続できない。
旧バトルの途中状態をV2へ変換する処理はない。旧版の対戦を終了させて切り替え、
V2では新しい試合を作成する。旧対戦中フラグが残った部屋はV2で再利用せず新しい部屋を使用する。

このワークスペースにUnityのC#ソースはないため、Unity側の差し替えと実機確認は対象外。
本番DBへの接続・マイグレーション、デプロイは自動実行していない。

2026-09-26のローカル検証では、全パッケージのテストとgRPC/gRPC-Web通信テストが成功。
その後追加した部屋削除後の再送テストを含む保存テストも成功したが、
通信テストの最終再実行はWindows Application Controlがテスト実行ファイルをブロックした。
制御を回避する変更は行っていない。実MySQL/MariaDBの任意テストとUnity実機テストは未実施。

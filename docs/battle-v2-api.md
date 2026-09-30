# Auxilia V2 APIリファレンス

更新日: 2026-09-30。現在のproto・ハンドラー・保存処理に基づく。
構造化演出イベントの詳細は [演出イベント仕様](battle-v2-presentation.md) を参照。
設計・導入手順は [battle-v2.md](battle-v2.md) を参照。

## 1. 通信と共通仕様

### サービス名

| 完全修飾サービス名 | 用途 | RPC数 |
| --- | --- | --- |
| `game.network.v2.BattleServiceV2` | 認証、編成、戦闘、状態・履歴配信 | 14 |
| `game.network.v2.RoomServiceV2` | 入退室、対戦枠、ルーム準備・配信 | 9 |
| `game.network.v2.RoomMatchServiceV2` | 部屋作成・一覧・名称変更 | 3 |
| `user.UserService` | 既存アカウント管理。V2でも一部を利用 | 7 |

gRPC／gRPC-Webで呼び出す。RESTのJSON APIではない。
RPCパスは `/<完全修飾サービス名>/<メソッド名>`。
例: `/game.network.v2.BattleServiceV2/ApplyMove`。
接続先ホストは配置環境による。ポートはサーバーの `PORT` 設定、既定8080。

特記のないRPCはUnary（要求1件→応答1件）。
`StreamGame` はserver streaming、`StreamRoom` はbidirectional streaming。
V2のC#サービス・戦闘メッセージ名前空間は `Game.Network.V2`。
ルーム系の要求・応答には旧 `room.proto`／`room_match.proto` のメッセージを利用する。

旧BattleService・RoomService・RoomMatchServiceはソースを残しているが、現在の起動構成では未登録。
旧戦闘・旧ロビーのRPCは `UNIMPLEMENTED` になる。

### 認証

V2では `BattleServiceV2/Login` 以外の全RPCに、以下のmetadataを1件付ける。

```text
authorization: Bearer <Loginで取得したtoken>
```

`Bearer ` は大文字小文字を含めてこの形式を使う。トークンは現在64文字の16進文字列で、有効期限は24時間。
操作主体はトークンから決定する。戦闘の要求に `player_id` を指定するフィールドはない。
ルーム操作で送る `user_id` は、Loginの `player_id` と一致させる。
ストリームにも開始時にmetadataを付け、期限切れ時は再ログインして接続し直す。

既存 `UserService` の認証要否は第7節を参照。

### 型・省略・ID

- 以下のフィールド名はprotoの `snake_case`。C#生成コードでは対応するプロパティを使う。
- `T[]` は `repeated T`、`map<K,V>` はprotobufのmap。
- proto3のスカラー省略値は文字列`""`、整数`0`、bool`false`。省略と0/falseを区別しない。
- 「必須」はアプリケーション上の要求条件。proto3の `required` 宣言ではない。
- 要求例のJSONはフィールドを説明するためのもの。grpcurl等で使う場合はprotobuf JSONの規則に従う。
  `uint64` はJSONでは文字列で表せる。C#では生成された64bit整数型で扱う。
- `room_id`: 数値のルームID。戦闘側は`uint32`、ルーム側は`int32`。作成操作で扱う有効範囲は1〜2147483647。
- `match_id`: 試合ごとのUUID文字列。再戦時は別ID。
- `player_id`／`user_id`／`owner_id`: アカウントのUUID文字列。
- `definition_id`: キャラクター定義ID（例`zina`）。旧数値IDではない。
- `character_id`: 試合内の個体ID（例`p1-c1`）。定義IDと区別する。
- 日時は文字列。RFC3339形式、必要に応じて小数秒を含む。未設定の戦闘期限は
  Goのゼロ時刻 `0001-01-01T00:00:00Z` になる場合がある。

## 2. 最短の呼び出し順

1. 必要なら `UserService/CreateUser` でアカウント作成。
2. 両者が `BattleServiceV2/Login` → tokenとplayer_idを取得。
3. `RoomMatchServiceV2/CreateRoomMatch` で部屋を作成。作成者は自動入室。
4. 相手が `RoomServiceV2/JoinRoom`。
5. 両者が `EnterRing`、続いて `SetReady { ready: true }`。
6. いずれかが `BattleServiceV2/CreateGame`。代わりに `RoomServiceV2/StartMatch` も利用可能。
7. 相手は `GetRoomGame` で同じmatch_idを取得。
8. 両者が `RegisterCharacters` で3体登録。
9. 両者が `BattleServiceV2/Ready`。ここで戦闘開始。
10. `StreamGame` を受信しながら `ApplyMove`／`ApplyAttack`／`EndTurn` を送信。
11. `finished=true` を受けて結果表示。降参は `Surrender`。
12. 再戦はルームの `SetReady` から繰り返す。

**ルームのSetReadyと戦闘のReadyは別操作。**
SetReadyだけでは試合を作らず、CreateGameだけでは戦闘を始めない。

## 3. BattleServiceV2

### 3.1 Login

`Login(LoginRequest) → LoginResponse`。認証metadata不要。

| 要求フィールド | 型 | 条件 |
| --- | --- | --- |
| `name` | string | 必須。既存アカウント名。最大128バイト |
| `password` | string | 必須。最大72バイト |

| 応答フィールド | 型 | 内容 |
| --- | --- | --- |
| `token` | string | 後続RPCで使うセッショントークン |
| `player_id` | string | 認証した本人のアカウントID |
| `expires_at` | string | セッション有効期限（UTC） |

入力不備、ユーザー不在、パスワード不一致は `UNAUTHENTICATED`。
既存 `UserService/Login` はトークンを発行しないため、このRPCの代用にはならない。

### 3.2 GetDefinitions

`GetDefinitions(Empty) → DefinitionsResponse`。要求フィールドなし。

| 応答フィールド | 型 | 内容 |
| --- | --- | --- |
| `definitions_json` | string | CharacterDefinition配列をJSON化した**文字列**。クライアントで別途JSON解析する |
| `rules_version` | string | 現在 `web-2026-09-26-v2` |

定義JSONの構造は第6節。編成画面・技説明などに利用する。

### 3.3 CreateGame

`CreateGame(CreateGameRequest) → Snapshot`。

| 要求フィールド | 型 | 条件 |
| --- | --- | --- |
| `room_id` | uint32 | 必須。既存ルームID |

新規作成時は1P/2Pが別ユーザーで埋まり、両者のルームReadyがtrueであること。
呼び出せるのはその対戦参加者。試合作成と同時に `is_gaming=true` にする。
戻る状態は `started=false`、`phase="waiting"`。編成はまだ未登録。
同じルームに未終了のV2試合があれば、参加者にはその試合を返す。

主な失敗: 部屋なし=`NOT_FOUND`、準備不足・旧試合等で対戦中=`FAILED_PRECONDITION`、非参加者=`PERMISSION_DENIED`。

### 3.4 GetRoomGame

`GetRoomGame(CreateGameRequest) → Snapshot`。
要求は `room_id: uint32`。試合作成は行わない。

指定ルームの未終了試合を優先し、なければ終了済み試合を作成日時の新しい順で取得する。
その試合の参加者、または現在そのルームに所属するユーザーが取得可能。
試合がない場合は `NOT_FOUND`。部屋一覧からの復帰・観戦・相手側のmatch_id取得に使う。

### 3.5 RegisterCharacters

`RegisterCharacters(SelectionRequest) → Snapshot`。

| 要求フィールド | 型 | 条件 |
| --- | --- | --- |
| `match_id` | string | 必須 |
| `definition_ids` | string[] | 必須。定義済みIDを重複なしでちょうど3体 |

認証した本人の編成を登録する。開始前・未終了の試合だけで利用可能。
配列順が初期配置順になる。1Pは `(0,4),(1,2),(0,0)`、2Pは `(7,4),(6,2),(7,0)`。
再登録すると状態を再構成し、**両者の戦闘Readyを解除する**。同じ内容の再送でもこの処理を行う。
旧綴り`wellbulus`→`verbulus`、`shincho`→`shicho`を正規化し、正規化後に重複を検査する。

```json
{"match_id":"<match UUID>","definition_ids":["zina","jude","dana"]}
```

登録数・ID不正=`INVALID_ARGUMENT`、開始後・終了後=`FAILED_PRECONDITION`。

### 3.6 Ready

`Ready(GameRequest) → Snapshot`。要求は `match_id: string`。

参加者本人を戦闘準備完了にする。両者の3体登録が必要。
1人目は待機継続。2人目でパッシブ処理と120秒タイマーを開始し、`started=true`、`phase="action"`になる。
同じ参加者の再送はReadyを重複追加しない。開始済みでも未終了なら状態を返す。
ただし保存処理のログ番号は増える場合がある。
編成未完了・終了済みは `FAILED_PRECONDITION`。

### 3.7 CancelGame

`CancelGame(GameRequest) → Snapshot`。要求は `match_id: string`。

参加者が開始前の試合全体を中止する。自分のReadyだけを取り消すAPIではない。
`finished=true`、勝者なしとなり、ルームの対戦中フラグとReadyを解除する。レート・対戦数は更新しない。
開始後は `FAILED_PRECONDITION`。開始前に中止済みの試合への再送は成功する。

### 3.8 GetGameData

`GetGameData(GameRequest) → Snapshot`。要求は `match_id: string`。

最新の保存状態を取得する。参加者または現在そのルームに所属するユーザーが利用可能。
参加者は終了後・ルーム退出後も試合IDで取得可能。読み取り自体はターンを進めない。
期限処理は別ワーカーが行うため、期限をわずかに過ぎた保存状態が返る場合もある。

### 3.9 StreamGame

`StreamGame(GameRequest) → stream Snapshot`。要求は `match_id: string`、権限はGetGameDataと同じ。

- 接続時に最新Snapshotを1件送る。
- 約250msごとに確認し、`last_log_sequence` が変化したときに最新Snapshotを送る。
- すべての中間Stateを1件ずつ送る保証はないが、直近32遷移の演出イベントを同梱する。
  同梱範囲より古い欠落分だけFetchActionLogで補う。
- 終了済みSnapshotを送信するとストリームは正常終了する。
- クライアントのキャンセル・接続断・認証期限切れ・権限喪失等でも終了する。
- 無変化時の時刻通知はない。画面の残り時間は最後の `server_time` と期限から表示する。

### 3.10 操作用の共通要求 ActionRequest

以下の4RPCはすべて `ActionRequest → Snapshot`。

| フィールド | 型 | ApplyMove | ApplyAttack | EndTurn / Surrender |
| --- | --- | --- | --- | --- |
| `match_id` | string | 必須 | 必須 | 必須 |
| `command_id` | string | 必須。1〜128バイト | 同左 | 同左 |
| `expected_revision` | uint64 | 必須。最新state.revision | 同左 | 同左 |
| `character_id` | string | 必須。自身の生存個体ID | 同左 | 未使用 |
| `attack_index` | int32 | 未使用 | 技番号0〜2。0も有効 | 未使用 |
| `target` | Position | 必須。移動先 | 必須。技の対象座標 | 未使用 |
| `direction` | Position | 未使用 | 必須。上下左右の単位方向 | 未使用 |

未使用フィールドは省略する。指定すると、処理済みコマンドの内容比較にも含まれるため、再送時に変更しない。
Positionは `x:int32, y:int32`。盤面座標は `0≤x<8`、`0≤y<5`。
技のdirectionは `(1,0),(-1,0),(0,1),(0,-1)` のみ。Unityの画面座標への変換はクライアント側で行う。

#### ApplyMove

自身の手番・操作フェーズに、自身の生存キャラクターを移動する。
距離・占有・障害物・敵拠点・効果・コストをサーバーが検査し、移動先地形効果も処理する。

```json
{"match_id":"<match UUID>","command_id":"<command UUID>","expected_revision":"6","character_id":"p1-c1","target":{"x":1,"y":4}}
```

#### ApplyAttack

自身の手番・操作フェーズに技を使用する。
形態に応じた技定義、範囲、対象、威力、コスト、効果、固有能力、勝敗をサーバーが決定する。
自分対象の技ではtargetに自分の現在座標を指定する。
targetは技を成立させる指定座標であり、範囲技の被弾者を1体に限定する指定ではない。

```json
{"match_id":"<match UUID>","command_id":"<command UUID>","expected_revision":"7","character_id":"p1-c1","attack_index":0,"target":{"x":3,"y":2},"direction":{"x":1,"y":0}}
```

例のrevision・座標は説明用。実際の状態・定義に合わせる。

#### EndTurn

自身の手番を終了する。終了効果をサーバーが処理する。
勝敗が決まらなければ `phase="turn_end"` となり、通常2秒後にワーカーが次手番へ進める。
クライアントからNewTurnを送る必要はない。終了効果で勝敗確定した場合はfinishedがtrueになる。

#### Surrender

開始済み・未終了の試合を降参する。自身の手番でなくても利用でき、`turn_end` フェーズでも可能。
revisionは一致が必要。相手を勝者として即時終了し、レート・対戦数・勝利数とルーム状態を更新する。

```json
{"match_id":"<match UUID>","command_id":"<command UUID>","expected_revision":"8"}
```

この形式はEndTurnとSurrenderで共通。

### 3.11 FetchActionLog

`FetchActionLog(LogRequest) → LogResponse`。権限はGetGameDataと同じ。

| 要求フィールド | 型 | 内容 |
| --- | --- | --- |
| `match_id` | string | 必須 |
| `after_sequence` | uint64 | この番号**より後**を取得。先頭からは0 |
| `limit` | uint32 | 1〜100。0または100超は100として扱う |

| 応答フィールド | 型 | 内容 |
| --- | --- | --- |
| `logs` | ActionLog[] | sequence昇順の保存済み遷移 |
| `next_sequence` | uint64 | 最後に返したsequence。0件なら要求after_sequenceと同値 |

ActionLog:

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `sequence` | uint64 | 試合内の永続ログ番号。1から始まる |
| `player_id` | string | 操作者。試合作成・タイマーでは空 |
| `action_type` | string | `CREATED`, `SELECT`, `READY`, `CANCEL`, `MOVE`, `ATTACK`, `END_TURN`, `SURRENDER`, `TIMER` |
| `command` | ActionRequest | 4種の操作RPCの要求情報。それ以外では未設定 |
| `before` | State | 遷移前状態。CREATEDでは未設定 |
| `after` | State | 遷移後状態 |
| `presentation` | PresentationBatch | 同じ遷移の構造化演出イベント。導入前の履歴では未設定 |

`before`／`after`は保存時点の状態で、`server_time`は現在時刻を表すものではない。
次のページは `after_sequence=next_sequence` として取得し、logsが空なら追いついている。

## 4. RoomMatchServiceV2

### 4.1 CreateRoomMatch

`CreateRoomMatch(roommatch.CreateRoomMatchRequest) → roommatch.RoomMatchResponse`。

| 要求フィールド | 型 | 条件 |
| --- | --- | --- |
| `room_name` | string | 必須。空白だけは不可、最大128バイト |
| `owner_id` | string | 必須。認証した本人のID |
| `is_gaming` | bool | falseにする。省略可能 |

応答は `room: RoomMatch`。作成者をstate=0の観戦枠で自動入室させる。
同じ要求の再送で別の部屋を作成するため、操作RPCのcommand_id方式の重複防止はない。
owner_id不一致・trueのis_gaming・不正な名前は `INVALID_ARGUMENT`。

### 4.2 ListRoomMatch

`ListRoomMatch(roommatch.ListRoomMatchRequest) → roommatch.ListRoomMatchResponse`。
要求フィールドなし。応答は `rooms: RoomMatch[]`。ルームID昇順。ページングなし。

### 4.3 UpdateRoomMatch

`UpdateRoomMatch(roommatch.UpdateRoomMatchRequest) → roommatch.RoomMatchResponse`。

| 要求フィールド | 型 | 条件 |
| --- | --- | --- |
| `room_id` | int32 | 必須。変更対象 |
| `room_name` | string | 必須。空白だけ不可、最大128バイト |
| `owner_id` | string | 必須。現在の所有者かつ認証本人 |
| `is_gaming` | bool | false。省略可能 |

部屋名を変更し、更新後の `room: RoomMatch` を返す。
所有者の譲渡や対戦中フラグの操作には使えない。
所有者不一致=`PERMISSION_DENIED`、対戦準備中・対戦中／is_gaming=trueの要求=`FAILED_PRECONDITION`。

RoomMatch:

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `room_id` | int32 | ルームID |
| `room_name` | string | 部屋名 |
| `owner_id` | string | 所有者ID |
| `is_gaming` | bool | V2試合作成から終了までtrue。戦闘Ready前もtrue |

## 5. RoomServiceV2

### 共通応答・条件

全メソッドが `rooms: Room[]` を返す。これは部屋一覧ではなく、**指定部屋の所属ユーザー一覧**。
StartMatchのみ `started:bool` が追加される。配列の並び順に依存せず `state`／`user_id` で判定する。

| Roomフィールド | 型 | 内容 |
| --- | --- | --- |
| `room_id` | int32 | 所属ルームID |
| `user_id` | string | 所属ユーザーID |
| `state` | int32 | 0=観戦、1=1P、2=2P |
| `is_ready` | bool | ルームでの準備完了。戦闘のReadyとは別 |
| `joined_at` | string | 入室日時（RFC3339） |

JoinRoom・LeaveRoom・EnterRing・LeaveRing・SetReady・UpdateRoomStateは、
`room_id>0`、`user_id=認証本人` が必要。`is_gaming=true` の間はすべて拒否する。
枠変更で1P/2Pが重複した場合は `RESOURCE_EXHAUSTED`。

### 各メソッド

| RPC | 要求型 | 要求フィールド（すべてproto名） | 応答型 |
| --- | --- | --- | --- |
| `JoinRoom` | room.JoinRoomRequest | room_id:int32、user_id:string | room.JoinRoomResponse |
| `LeaveRoom` | room.LeaveRoomRequest | room_id:int32、user_id:string | room.LeaveRoomResponse |
| `ListRoom` | room.ListRoomRequest | room_id:int32 | room.ListRoomResponse |
| `EnterRing` | room.EnterRingRequest | room_id:int32、user_id:string | room.EnterRingResponse |
| `LeaveRing` | room.LeaveRingRequest | room_id:int32、user_id:string | room.LeaveRingResponse |
| `SetReady` | room.SetReadyRequest | room_id:int32、user_id:string、ready:bool | room.SetReadyResponse |
| `UpdateRoomState` | room.UpdateRoomStateRequest | room_id:int32、user_id:string、state:int32、is_ready:bool | room.UpdateRoomStateResponse |
| `StartMatch` | room.StartMatchRequest | room_id:int32 | room.StartMatchResponse |
| `StreamRoom` | stream room.RoomStreamRequest | room_id:int32、user_id:string | stream room.ListRoomResponse |

#### JoinRoom

観戦枠・Ready=falseで入室する。定員8名。満員は `RESOURCE_EXHAUSTED`。
同じユーザーが既に入室していても、既存所属を削除して観戦枠で入室し直す。
そのため、再送で対戦枠・Ready・入室時刻が変わることに注意する。

#### LeaveRoom

指定部屋から退出する。所有者が退出した場合は残ったユーザーへ所有権を移す。
最後の1人なら部屋も削除する。この場合roomsは空。

#### ListRoom

指定部屋の所属ユーザー一覧を取得する。認証は必要だが、現在の実装では本人の所属は要求しない。
該当する所属レコードがなければroomsは空。部屋自体の存在を別途検査するAPIではない。

#### EnterRing

観戦者を空いている対戦枠に移す。1Pが空なら1P、埋まっていれば2P。
両枠が埋まっていると `RESOURCE_EXHAUSTED`。
現在の実装では更新件数を検査しないため、未入室などで状態が変わらず成功する場合がある。
戻り値の自分のRoom.stateを確認する。

#### LeaveRing

対戦枠から観戦枠へ戻り、Readyをfalseにする。観戦中なら変更なし。
1Pが抜けた場合は2Pを1Pへ移すため、全員分の応答を反映する。

#### SetReady

対戦枠のユーザーのルームReadyを更新する。ready省略はfalse。
観戦者のSetReadyは `FAILED_PRECONDITION`。このRPCだけでは試合は作成されない。

#### UpdateRoomState

自身のstate（0〜2）とis_readyをまとめて更新する。どちらも省略時は0/falseで上書きする。
state範囲外は `INVALID_ARGUMENT`。指定した枠が重複すると `RESOURCE_EXHAUSTED`。
SetReadyと異なり、現在の実装では観戦者のis_ready=trueを個別に拒否しない。
試合作成には1P/2P両枠のReadyが必要なため、観戦者のReadyだけで試合は作成されない。

#### StartMatch

内部でBattleServiceV2/CreateGameと同じ試合作成処理を行う。既存の未終了V2試合があれば再利用する。
応答の `started=true` は**試合作成・確保の成功**であり、戦闘の `State.started` ではない。
match_idは返さないため、`BattleServiceV2/GetRoomGame` で取得する。

#### StreamRoom

最初の要求で監視するroom_idと本人user_idを送る。直後から約500msごとにRoom一覧を送信する。
戦闘ストリームと異なり、内容が変化しなくても配信する。
2件目以降の要求内容は読み捨てるため、同じストリームで監視部屋を変更できない。
クライアント送信側の正常終了（half-close）後もサーバーの配信は続く。
監視終了はストリーム自体をキャンセルする。認証期限切れ等でも終了する。
現実装では監視対象部屋への所属は検査しない。

## 6. 戦闘の共通応答型

### Snapshot

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `room_id` | uint32 | 試合が属するルームID |
| `state` | State | 戦闘状態全体 |
| `last_log_sequence` | uint64 | 最新の永続ログ番号。状態応答の新旧比較に使う |
| `selected_player_ids` | string[] | 3体の編成を登録済みの参加者ID |
| `p1_rate` / `p2_rate` | int32 | 結果確定後のレート。それまでは0 |
| `p1_rate_delta` / `p2_rate_delta` | int32 | その試合によるレート変動。未確定・開始前中止では0 |
| `rules_version` | string | ルール識別子 |
| `presentation_batches` | PresentationBatch[] | 直近最大32遷移の演出イベント。sequence昇順 |
| `presentation_from_sequence` | uint64 | 同梱範囲の先頭sequence。空ならlast_log_sequence+1 |

### State

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `match_id` | string | 試合ID |
| `revision` | uint64 | エンジン状態の版。次の操作のexpected_revisionに使う |
| `started` | bool | 戦闘を開始したか。終了後も開始済みならtrue |
| `ready_player_ids` | string[] | 戦闘Readyを送信した参加者 |
| `players` | Player[] | 2名。添字0が1P、1が2P |
| `bases` | Base[] | 2拠点。同じく1P/2P順 |
| `characters` | Character[] | 登録済みキャラクター。開始前は0〜6体 |
| `tile_effects` | TileEffect[] | 配置された地形効果 |
| `blocked_cells` | Position[] | 侵入不可能マス |
| `turn_player_id` | string | 現在の手番の参加者ID。1Pが必ず先手になるわけではない |
| `turn` | int32 | 手番番号。1から開始 |
| `phase` | string | `waiting`=準備、`action`=操作、`turn_end`=終了処理 |
| `phase_deadline` | string | 終了処理フェーズの期限 |
| `turn_deadline` | string | 操作フェーズの期限 |
| `server_time` | string | Snapshot作成時のサーバー時刻 |
| `winner_id` | string | 勝者ID。未決着・開始前中止なら空 |
| `finished` | bool | 試合終了または中止 |
| `last_event` | Event | エンジンの直近イベント |
| `events` | Event[] | エンジン内の最近のイベント。全履歴ではない |
| `test_owner_id` | string | 移植元のテストモード用。通常V2 APIで作成する試合は空 |

`phase="finished"` は存在しない。終了は必ずfinishedで判定する。
通常の操作期限は120秒、終了処理フェーズは2秒。ワーカーのチェック間隔は250msであり、
期限ちょうどに通知される保証はない。時間切れの確定もサーバー状態を優先する。

### Player / Base / Position

| 型 | フィールド | 型 | 内容 |
| --- | --- | --- | --- |
| Player | `id` | string | アカウントID |
| Player | `name` | string | 試合作成時の名前 |
| Player | `cost` | int32 | 現在コスト |
| Base | `owner_id` | string | 所有者ID |
| Base | `hp` | int32 | 現在耐久力 |
| Base | `max_hp` | int32 | 最大耐久力。現在400 |
| Base | `position` | Position | 拠点座標 |
| Position | `x` / `y` | int32 | 盤面座標、または方向・定義内の相対座標 |

BaseとCharacterの `max_hp` はprotobuf JSON名が特例で `maxHP`。

### Character

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `id` | string | 試合内個体ID。操作のcharacter_idに指定 |
| `definition_id` | string | 定義ID |
| `owner_id` | string | 所有プレイヤーID |
| `name` | string | キャラクター名 |
| `hp` / `max_hp` | int32 | 現在HP／最大HP |
| `position` | Position | 現在位置 |
| `effects` | string[] | 現在のバフ・デバフ名 |
| `revive_used` | bool | 復活能力を使用済みか |
| `departure_used` | bool | 撃破時能力を使用済みか |
| `drank_turn` | int32 | 飲酒バフの終了判定に使う手番記録 |
| `hangover_turn` | int32 | 二日酔いの発生予定手番 |
| `hangover_until` | int32 | 二日酔いの終了判定用手番 |
| `used_skills` | map<string,int32> | 技名→使用した手番。ターン内使用制限用 |
| `combat_stance` | bool | 臨戦状態 |
| `barrier_turn` | int32 | 結界に関する手番記録 |
| `wriggling` | bool | 睡魔のくねくね状態 |
| `temporary_buffs` | string[] | 一時バフの管理情報 |

これらの特殊状態はサーバーが管理し、クライアントから更新値を送らない。

### TileEffect / Event

| 型 | フィールド | 型 | 内容 |
| --- | --- | --- | --- |
| TileEffect | `position` | Position | 配置座標 |
| TileEffect | `type` | string | 地形効果名（例: 毒ガス、地雷） |
| TileEffect | `owner_id` | string | 配置者ID |
| TileEffect | `hp` | int32 | 効果が持つHP情報。不要な効果では0 |
| Event | `sequence` | uint64 | エンジンイベント番号。ActionLog.sequenceと別 |
| Event | `type` | string | エンジンイベント種別 |
| Event | `text` | string | 表示用メッセージ |

### definitions_jsonの中身

以下だけはprotobuf名ではなく、JSON文字列内のプロパティ名。

| CharacterDefinitionプロパティ | JSON型 | 内容 |
| --- | --- | --- |
| `id`, `name` | string | 定義ID、表示名 |
| `image`, `portrait` | string | 画像ファイル名。画像本体や配信URLではない |
| `maxHP` | number | 最大HP |
| `moveCost`, `moveRange` | number | 移動コスト・移動範囲 |
| `passiveName`, `passiveDescription` | string | パッシブ名・説明 |
| `attacks` | AttackDefinition[3] | 通常時の3技 |
| `alternateAttacks` | AttackDefinition[3] | 切替後の3技。対象キャラのみ存在 |

| AttackDefinitionプロパティ | JSON型 | 内容 |
| --- | --- | --- |
| `name` | string | 技名 |
| `cost`, `power`, `range` | number | コスト、威力、範囲の値。回復は負のpowerを使う場合がある |
| `target` | string | `enemy`, `ally`, `any`, `cell` |
| `pattern` | Position[] | 基準方向に対する相対座標の集合 |
| `effect`, `allyEffect` | string | 付与効果 |
| `effectChance` | number | 効果付与率（百分率） |
| `tile` | string | 設置する地形効果 |
| `oncePerTurn` | bool | 手番内1回の制限 |
| `clearDebuffs`, `clearBuffs` | bool | 解除処理の指定 |
| `description` | string | 技説明 |

任意プロパティは値が空・0・falseのとき省略される。
固有能力の全計算をこの定義JSONだけから再現する必要はなく、実際の判定はサーバーが行う。
現行の定義ID:
`suima`, `kasuima`, `verbulus`, `sophie`, `jude`, `nadia`, `tsukiha`, `aoi`,
`sena`, `berenice`, `chiyo`, `shicho`, `zina`, `dana`, `louise`, `liberette`。

## 7. 関連する既存UserService

以下は `user.UserService`。V2サービスの名前空間とは異なる。

| RPC | 要求型・フィールド | 応答 | 認証 |
| --- | --- | --- | --- |
| `CreateUser` | CreateUserRequest: name:string、password:string | UserResponse | 不要 |
| `Login` | user.LoginRequest: name:string、password:string | UserResponse（tokenなし） | 不要 |
| `GetUser` | GetUserRequest: id:string | UserResponse | 不要 |
| `GetUserByName` | NameRequest: name:string | UserResponse | 不要 |
| `ListUsers` | ListUsersRequest: フィールドなし | ListUsersResponse: users:UserResponse[] | 不要 |
| `UpdateUser` | UpdateUserRequest: 下表 | UserResponse | V2 token、本人のみ |
| `DeleteUser` | DeleteUserRequest: id:string | DeleteUserResponse: success:bool | V2 token、本人のみ |

CreateUserは名前必須・16文字以内、パスワード6バイト以上。bcryptの制約上72バイト以内を使用する。
GetUser/DeleteUser/UpdateUserのidはアカウントID。
DeleteUserは所属しているルームが1つでもあれば `FAILED_PRECONDITION`。
成功時は本人のセッションも削除する。

UpdateUserRequest:

| フィールド | 型 | 現在の動作 |
| --- | --- | --- |
| `id` | string | 必須。認証本人のID |
| `name` | string | 空でなければ変更 |
| `password` | string | 空でなければ変更。6バイト以上 |
| `story` | int32 | 0より大きいと変更 |
| `num_wins`, `num_battles`, `rate` | int32 | **V2ガードが無視する**。サーバーの確定値を維持 |
| `home_character_id` | int32 | 0以上なら変更。変更しない場合は-1 |
| `deck1`, `deck2`, `deck3` | int32 | -1以上なら変更。-1は未設定。変更しない場合は-2以下 |

省略した整数は0になるため、プロフィールの名前だけを変更する場合も
home_character_id=-1、deck1〜3=-2を明示して既存設定の上書きを避ける。
旧数値デッキとV2の文字列definition_idsを自動変換するAPIはない。

UserResponse:

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `id`, `name` | string | アカウントID・名前 |
| `story` | int32 | ストーリー進捗 |
| `num_wins`, `num_battles` | int32 | 勝利数・対戦数 |
| `rate` | int32 | 現在レート |
| `home_character_id` | int32 | 既存のホーム用キャラクター数値ID |
| `deck1`, `deck2`, `deck3` | int32 | 既存の数値デッキ設定 |

## 8. エラーと再送

失敗はgRPC statusで返す。通常の戦闘応答にsuccess/messageを付ける方式ではない。
gRPC-Webではトランスポートが返すgRPC statusを読み取り、HTTPステータスだけで判定しない。

| status | 主な原因 | クライアント側の対応 |
| --- | --- | --- |
| `UNAUTHENTICATED` | tokenなし、不正・期限切れ、Login失敗 | Loginし直す。認証内容を確認 |
| `PERMISSION_DENIED` | 本人ID不一致、非参加者による操作、閲覧権限なし | 対象・本人ID・所属を確認 |
| `INVALID_ARGUMENT` | 座標・技・編成・方向・command_id等が不正、実行不能な戦闘操作 | 状態・入力を確認 |
| `FAILED_PRECONDITION` | 手番違い、準備不足、ロビー変更不可、開始後キャンセル等 | 状態を取得し直して条件を確認 |
| `ABORTED` | expected_revisionが古い | 最新状態で操作を再評価し、新しいcommand_idで送る |
| `ALREADY_EXISTS` | 同じcommand_idを別内容で再利用 | 再送なら元の内容を維持。別操作なら新ID |
| `NOT_FOUND` | 試合・ルーム・必要レコードが存在しない | IDを再確認 |
| `RESOURCE_EXHAUSTED` | 満室、対戦枠が埋まっている・重複 | 空き枠や別ルームを選ぶ |
| `CANCELED` / `DEADLINE_EXCEEDED` | キャンセル・期限超過 | 成否不明なら同じcommand_idと内容で再送 |
| `INTERNAL` | DB・変換処理等の内部エラー | 成否を状態・履歴で確認 |
| `UNIMPLEMENTED` | 旧APIや未登録RPCを呼んだ | V2サービス名とメソッドを確認 |

これはV2ハンドラーの主な分類。複数条件に違反すると、先に検査された条件のエラーが返る。
既存UserServiceの未変更メソッドには従来のstatus分類も残る。

### 4種の戦闘操作の重複防止

重複の単位は `(match_id, 認証本人, command_id)`。
最初に成功した操作を保存し、同内容の再送では再適用せず**現在のSnapshot**を返す。
元の応答と同じrevision・状態とは限らない。期限処理が進んでいることもある。
失敗した操作は処理済み記録を作らないが、先に実行された期限処理だけは保存される場合がある。

この仕組みはApplyMove・ApplyAttack・EndTurn・Surrender専用。
RegisterCharactersやJoinRoom等を同じ感覚で無条件再送しない。

### 状態とログの番号

- `State.revision`: 次の操作に付ける版番号。1操作で複数増える場合がある。
- `Event.sequence`: エンジン内イベント番号。
- `ActionLog.sequence`／`Snapshot.last_log_sequence`: 永続化された遷移の番号。
  revisionと同じ値になるとは限らない。

Unary応答とストリームが前後して届く場合、同一match_id内で古いlast_log_sequenceの
Snapshotを反映しない。ログ取得にはrevisionではなくActionLog.sequenceを使う。
通常の演出は同梱のpresentation_batchesを使用し、最終状態はSnapshotへ同期する。
before/afterは状態の復旧・確認にも利用できる。イベント型・処理手順は
[演出イベント仕様](battle-v2-presentation.md)を参照。

## 9. 現在提供していない操作

- クライアントが計算したHP・地形・効果を反映する旧ApplyEffect／ApplyGridUpdate。
- 次手番をクライアントから始めるNewTurn。
- 戦闘Readyだけをfalseにする専用RPC（準備全体の中止はCancelGame）。
- 開始済み試合の編成変更、任意の状態上書き。
- Web版のHTTPマッチング・ゲスト・使用率集計・テストモード用API。
- token更新・Logoutの専用RPC。

## 10. 対応する実装ファイル

- [戦闘proto](../proto/v2/gameV2.proto)
- [ルームproto](../proto/v2/roomV2.proto)、[部屋管理proto](../proto/v2/roomMatchV2.proto)
- [ルーム共通メッセージ](../proto/room.proto)、[部屋管理共通メッセージ](../proto/room_match.proto)
- [ユーザーproto](../proto/user.proto)
- [戦闘ハンドラー](../handler/grpcv2/gameV2.go)
- [ルームハンドラー](../handler/grpcv2/roomV2.go)、[部屋管理ハンドラー](../handler/grpcv2/roomMatchV2.go)
- [ユーザー更新ガード](../handler/grpcv2/userGuardV2.go)
- [保存処理](../infrastructure/gormv2/gameV2.go)、[セッション処理](../infrastructure/gormv2/sessionV2.go)
- [エンジン](../domain/gamev2/gameV2.go)、[キャラクター定義](../domain/gamev2/charactersV2.go)

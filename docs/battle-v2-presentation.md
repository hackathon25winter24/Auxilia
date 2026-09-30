# V2 構造化演出イベント

更新日: 2026-09-30。

通常の戦闘では、Snapshotの `presentation_batches` を順に処理して演出する。
毎回FetchActionLogを呼んだり、Stateの差分から技を推測したりする必要はない。
最終的な盤面・HP・操作可否は引き続き `Snapshot.state` を正とする。

## 配信と履歴

既存のprotoフィールドは維持し、次を追加した。RPCの追加はない。

| 応答 | 追加フィールド | 意味 |
| --- | --- | --- |
| Snapshot | `presentation_batches: PresentationBatch[]` | 直近最大32件の状態遷移に対応するイベント。sequence昇順 |
| Snapshot | `presentation_from_sequence: uint64` | 同梱範囲の先頭sequence（含む）。空の場合はlast_log_sequence+1 |
| ActionLog | `presentation: PresentationBatch` | その履歴1件のイベント。導入前の履歴では未設定 |

Unary応答、GetGameData、GetRoomGame、StreamGame等、Snapshotを返すすべての経路で同じ形式。
StreamGameは以前と同様に最新状態を送るが、複数の操作が配信間隔内に進んでも、
32遷移以内なら途中のイベントを含む。イベントのない遷移も空のバッチとして同梱する。
初回接続・再接続でもこのウィンドウを返す。通常は初回の過去演出を再生せず、全状態を直接表示する。

32件はイベントの数ではなく**永続ログの遷移件数**。
ウィンドウから外れてもDBの遷移履歴には残る。必要なときだけFetchActionLogで取得する。
操作の再送は同じバッチを返し、新しい演出イベントを発生させない。
失敗した操作の演出は保存しない。ただし、同時に確定した期限処理は別のTIMERバッチとして残る。

## PresentationBatch

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `version` | uint32 | 現在1。演出イベント形式のバージョン |
| `sequence` | uint64 | ActionLog.sequenceと同じ遷移番号 |
| `before_revision` | uint64 | 遷移前のState.revision。作成前は0 |
| `after_revision` | uint64 | 遷移後のState.revision |
| `command_id` | string | 操作RPCのコマンドID。準備操作・自動処理等は空 |
| `action_type` | string | CREATED / SELECT / READY / CANCEL / MOVE / ATTACK / END_TURN / SURRENDER / TIMER |
| `events` | PresentationEvent[] | 解決順のイベント。空でもsequenceを処理済みに進める |

**一意キーは `(match_id, batch.sequence, event.index)`。**
Unary応答とStreamGame、履歴取得で同じイベントが届くため、このキーで二重再生を防ぐ。
単一の技の中でも、通常ヒット→追撃→撃破→復活はそれぞれ別イベントになる。
同時に変わる関連項目は記録上の順番で並ぶ。配列順はアニメーションの秒数や同時再生の指定ではない。

## PresentationEvent

フィールド名はproto名。C#は生成されたプロパティ名を使用する。

| フィールド | 型 | 内容 |
| --- | --- | --- |
| `index` | uint32 | バッチ内の0始まり連番 |
| `type` | string | 下記の演出種別 |
| `cause` | string | 発生理由。例ATTACK、FOLLOW_UP、POISON、MINE |
| `source_character_id` | string | 原因となった個体ID。環境要因・不明なら空 |
| `source_player_id` | string | 原因となったプレイヤー。地形では設置者。分からなければ空 |
| `target_kind` | string | CHARACTER / BASE / TILE / PLAYER / MATCH / CELL |
| `target_id` | string | CHARACTERは個体ID、BASEとPLAYERはプレイヤーID。TILEは設置者IDで、座標と併用する |
| `definition_id` | string | CHARACTER_SPAWNEDの定義ID |
| `skill_key` | string | 通常技は`定義ID/normal/技番号`、変化後は`定義ID/alternate/技番号`。技番号0始まり |
| `skill_name` | string | 使用時点で解決した技名 |
| `attack_index` | int32 | 技番号。技関連イベント以外では意味を持たない |
| `amount` | int32 | 実際に増減したHP等。ダメージ量・回復量は正数。オーバーキル分は含まない |
| `before_value`, `after_value` | int32 | HP、コスト、手番、形態フラグ等の変更前後 |
| `effect` | string | 効果名、地形名、ブロックした効果等 |
| `property` | string | 形態変更の対象。combat_stance / wriggling |
| `before_text`, `after_text` | string | フェーズ名、終了時の勝者ID等 |
| `from`, `to` | Position | 移動前後、技対象、地形座標等。意味のないフィールドは未設定 |
| `direction` | Position | SKILL_USEDの技方向 |

`cause` は、通常の操作種別に加えて次を使う。

| cause | 意味 |
| --- | --- |
| FOLLOW_UP | 同じ技の追撃 |
| KNOCKBACK | 技による強制移動 |
| POISON | ターン終了時の毒ダメージ |
| MINE / SPIKES / GAS | 地雷・まきびし・毒ガス |
| PASSIVE | パッシブによる回復・効果付与等 |
| REVIVE | 復活能力 |
| TURN_END / TURN_START | ターン境界の効果変更等 |
| TIMER | 期限監視による進行 |

毒の付与元は既存Stateに保存されていないため、毒ダメージのsourceは空。
sourceが空のイベントを、直前に操作したキャラクターの攻撃と推測しないこと。
地形効果には設置者IDを入れるが、設置した個体までは保存していない。

## 種別と主な表示処理

| type | 主なフィールド | 表示処理 |
| --- | --- | --- |
| SKILL_USED | source、skill_key/name、attack_index、to、direction | 技の発動モーション・効果音。通常の有効な技操作の先頭に1件 |
| TARGETED | target_id、source、cause | 通常攻撃ループで対象になった個体。ブロック・威力0でも発生し得る。これだけでHPを減らさない |
| DAMAGED | target_kind/id、amount、before/after_value | キャラ・拠点の被ダメージ表示 |
| HEALED | target_kind/id、amount、before/after_value | 回復表示 |
| DEFEATED | CHARACTERのtarget_id | キャラの撃破演出。その後REVIVEDが続く場合もある |
| REVIVED | target_id、amount、before/after_value | 復活演出とHP反映 |
| MOVED | target_id、from、to、cause | 通常移動／ノックバック演出 |
| EFFECT_ADDED / EFFECT_REMOVED | target_id、effect | 状態異常・バフの表示更新 |
| ATTACK_BLOCKED | target_id、effect | 結界による攻撃ブロック |
| EFFECT_BLOCKED | target_id、effect | 免疫による効果付与ブロック |
| FORM_CHANGED | target_id、property、before/after_value | 形態変更。値はfalse=0、true=1 |
| TILE_ADDED / TILE_REMOVED | to、effect、target_id | 地形の生成・消滅 |
| TILE_HP_CHANGED | to、effect、amount、before/after_value | 不変マスの耐久変化 |
| COST_CHANGED | PLAYERのtarget_id、before/after_value | コスト表示 |
| CHARACTER_SPAWNED | target_id、definition_id、to、after_value | 編成登録時の配置・定義変更。after_valueはHP |
| CHARACTER_REMOVED | target_id | 状態から削除された個体の表示解除 |
| MATCH_CREATED / MATCH_STARTED | target_kind=MATCH | 準備・開始表示 |
| TURN_CHANGED | PLAYERのtarget_id、before/after_value | 次の手番プレイヤー・手番番号表示 |
| PHASE_CHANGED | before/after_text | waiting / action / turn_endの表示切替 |
| MATCH_FINISHED | after_text | 勝者ID。開始前中止なら空 |

HPが変化しなければDAMAGED/HEALEDは出ない。
対象数の多い技ではSKILL_USEDを1件だけ再生し、その後に対象ごとのダメージ等を処理する。
拠点の破壊はBASEのDAMAGEDのafter_value=0で判断し、試合終了はState.finishedで最終確認する。
すべての固有パッシブに独立した「パッシブ発動」イベントを出すのではなく、
解決した効果にcauseとsourceを付ける。技対象の正確な結果には後続の各イベントを使う。
将来の未知のtype/causeは表示を省略しても状態の同期を継続する。

## Unity側の受信方針

1. 試合IDごとに、最新Stateと「演出キューへ登録済みのbatch.sequence」を別々に保持する。
2. 初回Snapshotは過去イベントを再生せず、Stateから全画面を構築する。
   登録済みsequenceをSnapshot.last_log_sequenceに合わせる。
3. 通常受信では、登録済み番号より後のバッチだけをsequence順に取り込む。
4. 各バッチのeventsをindex順にUnityのメインスレッド上の演出キューへ登録する。
   登録済みカーソルはキュー登録時に進め、再生完了まで待たない。これで配信重複を防ぐ。
5. Stateは新しいlast_log_sequenceのものを保持する。演出終了時に確定表示を整合させる。
   入力可否・次のexpected_revisionは演出中の仮の表示ではなく最新Stateに基づく。
6. `登録済みsequence+1 < presentation_from_sequence` のときだけ履歴不足。
   FetchActionLogで不足分をページ取得し、各ActionLog.presentationを同じキューへ渡す。
   この間も新しいSnapshotは保持するが、後続演出を欠落分より先に再生しない。
7. 履歴が導入前でpresentation未設定なら、途中の演出を作らず最新Stateへ直接同期する。
   未対応のversionの場合も同様に同期する。再接続で演出を省略する方針でもよい。

比較に使うのはbatch.sequenceであり、State.revisionではない。
1操作でrevisionが複数増えることがある。空のeventsのバッチもカーソルを進める。
同じSnapshotをUnaryとStreamで受けても、登録済みカーソル以下は再生しない。

## 保存・互換性・検証

- `battle_v2_matches.presentation_json`: 最新32バッチの配信用ウィンドウ。
- `battle_v2_transitions.presentation_json`: その遷移の永続イベント。
- 起動時AutoMigrateでカラム追加。既存のState JSONと文章Eventは変更しない。
- 状態、操作の処理済み記録、イベントは同じトランザクションで保存する。
- 導入前の履歴を推測でイベント化しない。導入後の遷移からバッチを生成する。
- エンジンの途中状態を記録する処理はrevision・乱数・計算結果に影響しない。
- protobufはフィールド追加のみ。旧クライアントは従来のStateを読める。
  演出イベントを利用するUnityクライアントではC#コードを再生成する。
- Unityのアニメーション再生処理自体は今回のサーバー実装に含まない。

テストは追撃→撃破→復活、ノックバック→地雷、回復→毒、形態変更、結界、
演出有無での計算結果一致、保存・再送・履歴・32件上限・導入前データ、
gRPC／gRPC-Web上の同梱とイベント同一性を対象にする。

関連: [APIリファレンス](battle-v2-api.md)、[proto](../proto/v2/gameV2.proto)、
[イベント収集処理](../domain/gamev2/presentationV2.go)。

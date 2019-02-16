# 目的
AWSでPackerでAMI作成、Ansibleでインスタンスのプロビジョニング、スクリプトで起動設定(LC)の作成、AutoScalingの起動設定の差し替えの処理を
CodePiplineで定期的におこなっている。この処理で、パイプラインを回すたびに、AMIとスナップショット、起動設定が作成される。
不要になったリソースを手動て定期的に削除するのは面倒で大変なため、Goでaws-sdk-goでコードを書き、Lambdaで実行する自動削除ツールを作った。

## 削除ツールの作りと実行条件

- aws-sdk-goを使ってAMIとSnapshotと起動設定を削除するツール
  - SSMパラメータに登録されている`_base_ami`のkye名が含まれるAMIは削除しない
  - AutoScalingGroupで保持している起動設定(LC)にセットされているAMIは削除しない
  - AutoSalingGroupで保持している起動設定(LC)は削除しない
  - 実行した日時より`AMIEXPIREDATE`と`LCEXPIREDATE`より前に作成されたものを削除する
  - 東京リージョンのLambdaで実行する
  - 結果をSlackに投稿する
  
- 処理を停止させる条件 (意図しない削除を防ぐため)
  - Slack関連の環境変数(SLACKURL/CHANNEL/USERNAME)以外の値がセットされていない場合
  - ssmパラメータからsource-amiが取得できなかった場合
  - AutoScalingGroup数より起動設定の除外リスト数が少なかった場合

#### ターミナルで実行した結果
- AWS Lambda実行する形に変えているので, `main(){}` に変更する必要があります。
![Alt Text](https://github.com/yhidetoshi/Pictures/raw/master/aws/terminal-deleteAMISnapshotLC2.png)



#### SSMパラメータのAMI-ID (削除しないAMI)

![Alt Text](https://github.com/yhidetoshi/Pictures/raw/master/aws/ssm-params-store.png)


- 環境変数( Lambdaで環境変数値をセットする )
```main.go
SLACKURL      = os.Getenv("SLACKURL")       // SlackのWebhookURL
CHANNEL       = os.Getenv("CHANNEL")        // Slackのチャンネル名
USERNAME      = os.Getenv("USERNAME")       // Slackに表示する名前
AMIEXPIREDATE = os.Getenv("AMIEXPIREDATE")  // AMIの有効期限
LCEXPIREDATE  = os.Getenv("LCEXPIREDATE")   // 起動設定の有効期限
SSMPAGE       = os.Getenv("SSMPAGE")        // 3 ( Page数 )
LCPAGE        = os.Getenv("LCPAGE")         // 4 ( Page数 )
```

## Lambdaへのデプロイ手順
```
$ make setup cross-build
$ zip -j deployment.zip ./build/pkg/main_linux_amd64/main
$ aws lambda update-function-code --function-name ${LAMBDA_FUNCTION_NAME} --zip-file fileb://deployment.zip --region ap-northeast-1

※ ${LAMBDA_FUNCTION_NAME}はLambdaで作ったfunction名
```

## Lambdaに必要な設定
- デプロイするLambda関数は事前に関数作成が必要
  - 関数名: deleteAMISnapshotLC
- ランタイムをGo 1.xを選択
- IAMロールを付与
  - AmazonEC2FullAccess
  - AutoScalingFullAccess
  - IAMReadOnlyAccess
  - AmazonSSMReadOnlyAccess
- タイムアウト時間は削除するリソース量に応じて設定する
- ハンドラは `main` にする。
- 環境変数を追加する。（設定する環境変数は以下に記載）
- Cloudwatch-Eventを連携させて定期実行させる


#### Lambdaでの環境変数値をセット

![Alt Text](https://github.com/yhidetoshi/Pictures/raw/master/aws/lambda-amideleteAMISnapshotLC.png)


#### Slack通知

![Alt Text](https://github.com/yhidetoshi/Pictures/raw/master/aws/slack-post-deletetool.png)


## まとめ
AWS-Lmabdaとaws-sdk-goを活用してインフラCI/CDで発生するリソースを定期的に削除するツールを作成しました。
リソースを削除するツールですので、もしご利用される場合は自己責任でお願いします。


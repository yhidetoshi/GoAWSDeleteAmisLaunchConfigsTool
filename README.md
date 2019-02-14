# GoAWSDeleteAmisLaunchConfigsTool

- aws-sdk-goを使ってAMIとSnapshotと起動設定を削除するツール
  - SSMパラメータに登録されている`_source_ami`のkye名が含まれるAMIは削除しない
  - AutoScalingGroupで保持している起動設定(LC)にセットされているAMIは削除しない
  - AutoSalingGroupで保持している起動設定(LC)は削除しない
  - 実行した日時より`AMIEXPIREDATE`と`LCEXPIREDATE`より前に作成されたものを削除する
  - 東京リージョンのLambdaで実行する
  - 結果をSlackに投稿する


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

#### Lambdaでの環境変数値をセット

![Alt Text](https://github.com/yhidetoshi/Pictures/raw/master/aws/lambda-amideleteAMISnapshotLC.png)


#### Slack通知

![Alt Text](https://github.com/yhidetoshi/Pictures/raw/master/aws/slack-post-deletetool.png)

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
SLACKURL      = os.Getenv("SLACKURL")
CHANNEL       = os.Getenv("CHANNEL")
USERNAME      = os.Getenv("USERNAME")
AMIEXPIREDATE = os.Getenv("AMIEXPIREDATE")
LCEXPIREDATE  = os.Getenv("LCEXPIREDATE")
SSMPAGE       = os.Getenv("SSMPAGE")
LCPAGE        = os.Getenv("LCPAGE")
```

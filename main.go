package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ashwanthkumar/slack-go-webhook"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/ssm"
)

const (
	SSMSEARCHWORD = "_base_ami"
)

var (
	config = aws.Config{Region: aws.String("ap-northeast-1")}
	svcEC2 = ec2.New(session.New(&config))
	svcSSM = ssm.New(session.New(&config))
	svcASG = autoscaling.New(session.New(&config))
	svcIAM = iam.New(session.New(&config))
	layout = "2006-01-02T15:04:05"

	SLACKURL      = os.Getenv("SLACKURL")
	CHANNEL       = os.Getenv("CHANNEL")
	USERNAME      = os.Getenv("USERNAME")
	AMIEXPIREDATE = os.Getenv("AMIEXPIREDATE")
	LCEXPIREDATE  = os.Getenv("LCEXPIREDATE")
	SSMPAGE       = os.Getenv("SSMPAGE")
	LCPAGE        = os.Getenv("LCPAGE")
)

// SSM Interface
type SSM interface {
	FetchSSMParamsKey()
	FetchSSMParamsValue(*string) string
	GetAMIFromSSM()
}

// ASG Interface
type ASG interface {
	FetchListLCNameCreatedTime()
	FetchLCName()
	FetchAMIFromLC()
	FetchThresholdLC()
	DeleteLCByLCName(string)
	DeleteLC()
}

// AMI Interface
type AMI interface {
	FetchListAMI()
	FetchSnapshotIDFromAMI([][]string)
	FetchThresholdAMI()
	DeleteSnapshotBySnapshotID()
	DeregisterAMIByAMIID(string)
}

// Slack Interface
type Slack interface {
	PostSlack()
	GetAccountAlias()
}

// SSMParams struct
type SSMParams struct {
	key      []string
	value    string
	amiIDSSM []string
}

// ASGParams struct
type ASGParams struct {
	lcNameFromASG     []string
	lcNameList        []string
	lcCreatedTimeList []string
	amiIDLCFromASG    []string
	threshold         string
	deleteCount       int
	deleteFlag        bool
}

// AMIParams struct
type AMIParams struct {
	amiList             [][]string
	snapshotID          string
	existSnapshots      int
	threshold           string
	excludedList        []string
	deleteFlag          bool
	deleteAMICount      int
	deleteSnapshotCount int
}

// SlackParams struct
type SlackParams struct {
	accountAlias string
	webhookurl   string
	channel      string
	username     string
}

func main() {
	lambda.Start(Handler)
}

func Handler(ctx context.Context) {

	// SSM
	sp := &SSMParams{}
	sp.FetchSSMParamsKey()
	sp.GetAMIFromSSM()
	fmt.Printf("SSM AMI List: \n%s\n\n", sp.amiIDSSM)

	// LauchConfig(LC)
	ap := &ASGParams{}
	ap.FetchLCName()
	ap.FetchAMIFromLC()
	fmt.Printf("LaunchConfig AMI List:\n%s\n\n", ap.amiIDLCFromASG)

	// AMI
	amip := &AMIParams{}
	amip.excludedList = append(sp.amiIDSSM, ap.amiIDLCFromASG...)
	fmt.Printf("Exculuded AMI List: \n%s\n\n", amip.excludedList)
	amip.FetchSnapshotList()
	amip.FetchListAMI()
	amip.FetchThresholdAMI()
	amip.DeleteAMISnapshot()

	// LaunchConfig Delete
	ap.FetchListLCNameCreatedTime()
	ap.FetchThresholdLC()
	fmt.Printf("Exculuded LaunchConfig List: \n%s\n\n", ap.lcNameFromASG)
	ap.DeleteLC()

	// Slack
	s := &SlackParams{}
	s.GetAccountAlias()
	excludedListAMISSM := strings.Join(sp.amiIDSSM, " / ")
	excludedListAMILC := strings.Join(ap.amiIDLCFromASG, " / ")
	excludedListLC := strings.Join(ap.lcNameFromASG, " / ")
	s.PostSlack(s.accountAlias, len(amip.amiList), amip.deleteAMICount, amip.existSnapshots, amip.deleteSnapshotCount, len(ap.lcNameList), ap.deleteCount, excludedListAMISSM, excludedListAMILC, excludedListLC)
}

// Snapshotのリストを返す
func (amip *AMIParams) FetchSnapshotList() {
	var owner, snapshotID []*string
	var _owner = []string{"self"}
	owner = aws.StringSlice(_owner)
	params := &ec2.DescribeSnapshotsInput{
		SnapshotIds: snapshotID,
		OwnerIds:    owner,
	}
	res, err := svcEC2.DescribeSnapshots(params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	amip.existSnapshots = len(res.Snapshots)
	//fmt.Println(res)
}

// AMI削除の閾値を取得
func (amip *AMIParams) FetchThresholdAMI() {

	// convert string to int
	var _AMIEXPIREDATE int
	_AMIEXPIREDATE, _ = strconv.Atoi(AMIEXPIREDATE)

	// 環境変数をセットしていなければ処理停止
	if _AMIEXPIREDATE == 0 {
		fmt.Println("Dont set AMIEXPIREDATE value")
		os.Exit(1)
	}

	t := time.Now()
	thresholdTime := t.AddDate(0, 0, _AMIEXPIREDATE)
	amip.threshold = thresholdTime.Format(layout)
}

// AMIのリストを取得
func (amip *AMIParams) FetchListAMI() {
	var owner, images []*string
	var _owner = []string{"self"}

	owner = aws.StringSlice(_owner)
	params := &ec2.DescribeImagesInput{
		ImageIds: images,
		Owners:   owner,
	}

	res, err := svcEC2.DescribeImages(params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	//fmt.Println(res)
	for _, resImage := range res.Images {

		amiInfo := []string{
			*resImage.ImageId,
			*resImage.CreationDate,
		}
		amip.amiList = append(amip.amiList, amiInfo)
	}
}

// AMI-IDでSnapshot-IDを取得
func (amip *AMIParams) FetchSnapshotIDFromAMI(amiID string) {
	params := &ec2.DescribeImagesInput{
		ImageIds: []*string{
			aws.String(amiID),
		},
	}
	res, err := svcEC2.DescribeImages(params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	for _, resImage := range res.Images {
		for _, resBlock := range resImage.BlockDeviceMappings {
			if resBlock.Ebs == nil {
				continue
			}
			amip.snapshotID = *resBlock.Ebs.SnapshotId
			//fmt.Println(amip.snapshotID)
		}
	}
}

// AMIとSnapshotの削除リクエスト
func (amip *AMIParams) DeleteAMISnapshot() {

	fmt.Println("AMI\t\t\tCreateDated\t\t\tSnapshot")
	fmt.Println("---------------------------------")
	for i := range amip.amiList {

		if amip.amiList[i][1] < amip.threshold {
			for j := range amip.excludedList {
				if amip.amiList[i][0] == amip.excludedList[j] {
					//fmt.Println(amip.list[i][1])
					amip.deleteFlag = false
				}
			}
			// 不要なAMIを削除 deleteCheckFlag is true -> delete
			// Deregister AMIs and Delete Snapshots
			if amip.deleteFlag == true {
				// 実行順注意(FetchしてAMI削除、Snapshot削除の順番)
				amip.FetchSnapshotIDFromAMI(amip.amiList[i][0])
				amip.DeregisterAMIByAMIID(amip.amiList[i][0])
				amip.DeleteSnapshotBySnapshotID(amip.snapshotID)
				fmt.Printf("%s\t%s\t%s\n", amip.amiList[i][0], amip.amiList[i][1], amip.snapshotID)
			}
			amip.deleteFlag = true
		}
	}
	fmt.Println("---------------------------------\n")
	fmt.Printf("Number of Deleted AMIs and Snapshots: %d\n\n", amip.deleteAMICount)
}

// AMI-IDを指定してAMIの登録解除
func (amip *AMIParams) DeregisterAMIByAMIID(deleteID string) {

	var _deleteID *string
	_deleteID = &deleteID

	params := &ec2.DeregisterImageInput{
		ImageId: _deleteID,
	}
	_, err := svcEC2.DeregisterImage(params)
	amip.deleteAMICount++
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// Snapshot-IDを指定して削除処理
func (amip *AMIParams) DeleteSnapshotBySnapshotID(_snapshotID string) {
	params := &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(_snapshotID),
	}
	_, err := svcEC2.DeleteSnapshot(params)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		return
	}
	amip.deleteSnapshotCount++
	//fmt.Println(_snapshotID)
}

// LaunchConfigの削除期間の閾値を設定
func (ap *ASGParams) FetchThresholdLC() {

	// convert string to int
	var _LCEXPIREDATE int

	_LCEXPIREDATE, _ = strconv.Atoi(LCEXPIREDATE)
	// 環境変数をセットしていなければ処理停止
	if _LCEXPIREDATE == 0 {
		fmt.Println("Dont set LCEXPIREDATE value")
		os.Exit(1)
	}

	t := time.Now()
	thresholdTime := t.AddDate(0, 0, _LCEXPIREDATE)
	ap.threshold = thresholdTime.Format(layout)
}

// LaunchConfig名を指定して削除処理
func (ap *ASGParams) DeleteLCByLCName(lcName string) {
	params := &autoscaling.DeleteLaunchConfigurationInput{
		LaunchConfigurationName: aws.String(lcName),
	}
	_, err := svcASG.DeleteLaunchConfiguration(params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	ap.deleteCount++
}

// LaunchConfigの削除リクエスト
func (ap *ASGParams) DeleteLC() {
	fmt.Println("LaunchConfig\t\t\t\tCreateDate")
	fmt.Println("---------------------------------")

	// 除外リストを作成できないので処理停止
	if len(ap.lcNameFromASG) == 0 {
		fmt.Println("none value error")
		os.Exit(1)
	}

	for i := range ap.lcNameList {
		if ap.lcCreatedTimeList[i] < ap.threshold {
			for j := range ap.lcNameFromASG {
				if ap.lcNameList[i] == ap.lcNameFromASG[j] {
					ap.deleteFlag = false
				}
			}
			if ap.deleteFlag == true {
				fmt.Printf("%s\t%s\n", ap.lcNameList[i], ap.lcCreatedTimeList[i])
				ap.DeleteLCByLCName(ap.lcNameList[i])
			}
			ap.deleteFlag = true
		}
	}
	fmt.Println("---------------------------------\n")
	fmt.Printf("Number of Deleted LCs: %d\n", ap.deleteCount)
}

// LaunchConfigのAMI-IDを取得
func (ap *ASGParams) FetchAMIFromLC() {

	// convert string to int
	var _LCPAGE int
	_LCPAGE, _ = strconv.Atoi(LCPAGE)

	// 環境変数をセットしていなければ処理停止
	if _LCPAGE == 0 {
		fmt.Println("Dont set LCPAGE value")
		os.Exit(1)
	}

	pageNum := 0

	params := &autoscaling.DescribeLaunchConfigurationsInput{}
	err := svcASG.DescribeLaunchConfigurationsPages(params,
		func(page *autoscaling.DescribeLaunchConfigurationsOutput, lastPage bool) bool {
			for i := range ap.lcNameFromASG {
				for _, value := range page.LaunchConfigurations {
					if ap.lcNameFromASG[i] == *value.LaunchConfigurationName {
						ap.amiIDLCFromASG = append(ap.amiIDLCFromASG, *value.ImageId)
					}
				}
			}
			pageNum++
			return pageNum <= _LCPAGE
		})
	// ASGの数より起動設定(LC)の数が少ないと処理停止
	if len(ap.lcNameFromASG) > len(ap.amiIDLCFromASG) {
		fmt.Println("cannot get all ami-id from lc")
		os.Exit(1)
	}

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// LaunchConfigのリストと作成日時を取得
func (ap *ASGParams) FetchListLCNameCreatedTime() {

	// convert string to int
	var _LCPAGE int
	_LCPAGE, _ = strconv.Atoi(LCPAGE)

	var lcTypeTime time.Time
	var lcTypeStr string
	pageNum := 0

	params := &autoscaling.DescribeLaunchConfigurationsInput{}
	err := svcASG.DescribeLaunchConfigurationsPages(params,
		func(page *autoscaling.DescribeLaunchConfigurationsOutput, lastPage bool) bool {
			for _, resLc := range page.LaunchConfigurations {
				ap.lcNameList = append(ap.lcNameList, *resLc.LaunchConfigurationName)

				// Convert *time.Time to string
				lcTypeTime = *resLc.CreatedTime
				lcTypeStr = lcTypeTime.Format(layout)
				ap.lcCreatedTimeList = append(ap.lcCreatedTimeList, lcTypeStr)
			}
			pageNum++
			return pageNum <= _LCPAGE
		})

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// LaunchConfig名を取得
func (ap *ASGParams) FetchLCName() {

	params := &autoscaling.DescribeAutoScalingGroupsInput{}
	res, err := svcASG.DescribeAutoScalingGroups(params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	for _, resASG := range res.AutoScalingGroups {
		ap.lcNameFromASG = append(ap.lcNameFromASG, *resASG.LaunchConfigurationName)
	}
}

// SSMパラメータからSourceAMI-IDを取得
func (sp *SSMParams) GetAMIFromSSM() {
	for i := range sp.key {
		sp.amiIDSSM = append(sp.amiIDSSM, sp.FetchSSMParamsValue(&sp.key[i]))
	}
}

// SSMパラメータから `_source_ami` に該当するkeyを取得
func (sp *SSMParams) FetchSSMParamsKey() {
	pageNum := 0

	// convert string to int
	var _SSMPAGE int

	_SSMPAGE, _ = strconv.Atoi(SSMPAGE)
	// 環境変数をセットしていなければ処理停止
	if _SSMPAGE == 0 {
		fmt.Println("Dont set SSMPAGE value")
		os.Exit(1)
	}

	params := &ssm.DescribeParametersInput{}
	err := svcSSM.DescribeParametersPages(params,
		func(page *ssm.DescribeParametersOutput, lastPage bool) bool {
			for _, value := range page.Parameters {
				if strings.Contains(*value.Name, SSMSEARCHWORD) {
					sp.key = append(sp.key, *value.Name)
				}
			}
			pageNum++
			return pageNum <= _SSMPAGE
		})
	// 除外リストを作成できないので処理停止
	if len(sp.key) == 0 {
		fmt.Println("none key error")
		os.Exit(1)
	}
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

// SSMパラメータで指定したKeyのValueを返す
func (sp *SSMParams) FetchSSMParamsValue(sourceAMI *string) string {
	params := &ssm.GetParameterInput{
		Name: sourceAMI,
	}
	res, err := svcSSM.GetParameter(params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	sp.value = *res.Parameter.Value
	// 除外リストを作成できないので処理停止
	if len(sp.value) == 0 {
		fmt.Println("none value error")
		os.Exit(1)
	}
	return sp.value
}

// AccountAliasを取得
func (s *SlackParams) GetAccountAlias() {
	params := &iam.ListAccountAliasesInput{}
	res, err := svcIAM.ListAccountAliases(params)
	if err != nil {
		fmt.Println("Got error fetching parameter: ", err)
		fmt.Println(err.Error())
		os.Exit(1)
	}
	if res.AccountAliases == nil {
		s.accountAlias = "None"
	} else {
		s.accountAlias = *res.AccountAliases[0]
	}
}

// 結果をSlackに投稿
func (s *SlackParams) PostSlack(accountName string, existAMIs int, deleteAMIs int, existSnapshots int, deleteSnapshots int, existLCs int, deleteLCs int, excludedListAMISSM string, excludedListAMILC string, excludedListLC string) {
	field1 := slack.Field{Title: "Account", Value: accountName}
	field2 := slack.Field{Title: "Exist AMIs", Value: strconv.Itoa(existAMIs)}
	field3 := slack.Field{Title: "Deleted AMIs", Value: strconv.Itoa(deleteAMIs)}
	field4 := slack.Field{Title: "Exist Snapshots", Value: strconv.Itoa(existSnapshots)}
	field5 := slack.Field{Title: "Deleted Snapshots", Value: strconv.Itoa(deleteSnapshots)}
	field6 := slack.Field{Title: "Exist LCs", Value: strconv.Itoa(existLCs)}
	field7 := slack.Field{Title: "Deleted LCs", Value: strconv.Itoa(deleteLCs)}
	field8 := slack.Field{Title: "Excluded AMIs List ( SSM )", Value: excludedListAMISSM}
	field9 := slack.Field{Title: "Excluded AMIs List ( LC )", Value: excludedListAMILC}
	field10 := slack.Field{Title: "Excluded LCs List", Value: excludedListLC}

	attachment := slack.Attachment{}
	attachment.AddField(field1).AddField(field2).AddField(field3).AddField(field4).AddField(field5).AddField(field6).AddField(field7).AddField(field8).AddField(field9).AddField(field10)
	color := "warning"
	attachment.Color = &color
	payload := slack.Payload{
		Username:    s.username,
		Channel:     s.channel,
		Attachments: []slack.Attachment{attachment},
	}
	err := slack.Send(SLACKURL, "", payload)
	if len(err) > 0 {
		os.Exit(1)
	}
}

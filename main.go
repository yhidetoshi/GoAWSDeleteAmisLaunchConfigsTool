package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ashwanthkumar/slack-go-webhook"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/autoscaling"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/iam"
	"github.com/aws/aws-sdk-go/service/ssm"
)

const (
	SSMSEARCHWORD = "_base_ami"
	SSMPAGE       = 3
	LCPAGE        = 4
	AMIDELETEDATE = -14
	LCDELETEDATE  = -14

	// Slack
	WEBHOOKURL = "https://hooks.slack.com/services/XXXXX"
	CHANNEL    = "dev"
	USERNAME   = "AutoDeleteAMIs"
)

var (
	config = aws.Config{Region: aws.String("ap-northeast-1")}
	svcEC2 = ec2.New(session.New(&config))
	svcSSM = ssm.New(session.New(&config))
	svcASG = autoscaling.New(session.New(&config))
	svcIAM = iam.New(session.New(&config))
	layout = "2006-01-02T15:04:05"
)

/* SSMパラメータ Interface */
type SSM interface {
	FetchSSMParamsKey()
	FetchSSMParamsValue(*string) string
	GetAMIFromSSM()
}

/* ASG Interface */
type ASG interface {
	FetchListLCNameCreatedTime()
	FetchLCName()
	FetchAMIFromLC()
	FetchThresholdLC()
	DeleteLCByLCName(string)
	DeleteLC()
}

/* AMI Interface */
type AMI interface {
	FetchListAMI()
	FetchSnapshotIDFromAMI([][]string)
	FetchThresholdAMI()
	DeleteSnapshotByID()
	DeregisterAMIByAMIID(string)
}

/* Slack Interface */
type Slack interface {
	PostSlack()
	GetAccountAlias()
}

/* SSMParamsの構造体 */
type SSMParams struct {
	key      []string
	value    string
	amiIdSSM []string
}

/* ASGParamsの構造体 */
type ASGParams struct {
	lcNameFromASG     []string
	lcNameList        []string
	lcCreatedTimeList []string
	amiIDLCFromASG    []string
	threshold         string
	deleteCount       int
	deleteFlag        bool
}

/* AMIParamsの構造体 */
type AMIParams struct {
	amiList      [][]string
	snapshotID   string
	threshold    string
	excludedList []string
	deleteFlag   bool
	deleteCount  int
}

/* SlackParamsの構造体 */
type SlackParams struct {
	accountAlias string
	webhookurl   string
	channel      string
	username     string
}

func main() {
	// SSM
	sp := &SSMParams{}
	sp.FetchSSMParamsKey()
	sp.GetAMIFromSSM()
	fmt.Printf("SSM AMI List: \n%s\n\n", sp.amiIdSSM)

	// LauchConfig(LC)
	ap := &ASGParams{}
	ap.FetchLCName()
	ap.FetchAMIFromLC()
	fmt.Printf("LaunchConfig AMI List:\n%s\n\n", ap.amiIDLCFromASG)

	// AMI
	amip := &AMIParams{}
	amip.excludedList = append(sp.amiIdSSM, ap.amiIDLCFromASG...)
	fmt.Printf("Exculuded AMI List: \n%v\n\n", amip.excludedList)
	amip.FetchListAMI()
	amip.FetchThresholdAMI()
	amip.DeleteAMISnapshot()

	// LaunchConfig Delete
	ap.FetchListLCNameCreatedTime()
	ap.FetchThresholdLC()
	ap.DeleteLC()

	// Slack
	s := &SlackParams{}
	s.GetAccountAlias()

	excludedListSSM := strings.Join(sp.amiIdSSM, " / ")
	excludedListLC := strings.Join(ap.amiIDLCFromASG, " / ")
	s.PostSlack(s.accountAlias, len(amip.amiList), amip.deleteCount, ap.deleteCount, excludedListSSM, excludedListLC)
}

/* AMIの削除期間の閾値を設定 */
func (amip *AMIParams) FetchThresholdAMI() {
	t := time.Now()
	thresholdTime := t.AddDate(0, 0, AMIDELETEDATE)
	amip.threshold = thresholdTime.Format(layout)
}

/* AMIのリストを取得 */
func (amip *AMIParams) FetchListAMI() {
	var owner, images []*string
	var _owner []string = []string{"self"}

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
	for _, resImage := range res.Images {

		amiInfo := []string{
			*resImage.ImageId,
			*resImage.CreationDate,
		}
		amip.amiList = append(amip.amiList, amiInfo)
	}
}

/* AMIからSnapshot-IDを取得 */
func (amip *AMIParams) FetchSnapshotIDFromAMI(amiId string) {
	params := &ec2.DescribeImagesInput{
		ImageIds: []*string{
			aws.String(amiId),
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

/* AMIとSnapshotの削除リクエスト */
func (amip *AMIParams) DeleteAMISnapshot() {

	amip.deleteCount = 0
	fmt.Println("AMI\t\t\tCreateDated\t\t\tSnapshot")
	fmt.Println("---------------------------------")
	for i, _ := range amip.amiList {

		if amip.amiList[i][1] < amip.threshold {
			for j, _ := range amip.excludedList {
				if amip.amiList[i][0] == amip.excludedList[j] {
					//fmt.Println(amip.list[i][1])
					amip.deleteFlag = false
				}
			}
			// 不要なAMIを削除 deleteCheckFlag is true -> delete
			if amip.deleteFlag == true {
				//amip.DeregisterAMIByAMIID(amip.amiList[i][0])
				amip.FetchSnapshotIDFromAMI(amip.amiList[i][0])
				//amip.DeleteSnapshotByID()
				fmt.Printf("%s\t%s\t%s\n", amip.amiList[i][0], amip.amiList[i][1], amip.snapshotID)
				amip.deleteCount++
			}
			amip.deleteFlag = true
		}
	}
	fmt.Println("---------------------------------\n")
	fmt.Printf("Number of Deleted AMIs and Snapshots: %d\n\n", amip.deleteCount)
}

/* AMI-IDを指定して登録解除処理 */
func (amip *AMIParams) DeregisterAMIByAMIID(deleteId string) {

	var _deleteId *string
	_deleteId = &deleteId

	params := &ec2.DeregisterImageInput{
		ImageId: _deleteId,
	}
	_, err := svcEC2.DeregisterImage(params)

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

/* Snapshot-IDを指定して削除処理 */
func (amip *AMIParams) DeleteSnapshotByID() {
	params := &ec2.DeleteSnapshotInput{
		SnapshotId: aws.String(amip.snapshotID),
	}
	_, err := svcEC2.DeleteSnapshot(params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

/* LaunchConfigの削除期間の閾値を設定 */
func (ap *ASGParams) FetchThresholdLC() {
	t := time.Now()
	thresholdTime := t.AddDate(0, 0, LCDELETEDATE)
	ap.threshold = thresholdTime.Format(layout)
}

/* LaunchConfig名を指定して削除処理 */
func (ap *ASGParams) DeleteLCByLCName(lcName string) {
	params := &autoscaling.DeleteLaunchConfigurationInput{
		LaunchConfigurationName: aws.String(lcName),
	}
	_, err := svcASG.DeleteLaunchConfiguration(params)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

/* LaunchConfigの削除リクエスト */
func (ap *ASGParams) DeleteLC() {
	fmt.Println("LaunchConfig\t\t\t\tCreateDate")
	fmt.Println("---------------------------------")

	for i, _ := range ap.lcNameList {
		if ap.lcCreatedTimeList[i] < ap.threshold {
			for j, _ := range ap.lcNameFromASG {
				if ap.lcNameList[i] == ap.lcNameFromASG[j] {
					ap.deleteFlag = false
				}
			}
			if ap.deleteFlag == true {
				fmt.Printf("%s\t%s\n", ap.lcNameList[i], ap.lcCreatedTimeList[i])
				// ap.DeleteLCByLCName(ap.lcNameList[i])
				ap.deleteCount++
			}
			ap.deleteFlag = true
		}
	}
	fmt.Println("---------------------------------\n")
	fmt.Printf("Number of Delete LCs: %d\n", ap.deleteCount)
}

/* LaunchConfigのAMI-IDを取得 */
func (ap *ASGParams) FetchAMIFromLC() {
	pageNum := 0

	params := &autoscaling.DescribeLaunchConfigurationsInput{}
	err := svcASG.DescribeLaunchConfigurationsPages(params,
		func(page *autoscaling.DescribeLaunchConfigurationsOutput, lastPage bool) bool {
			for i, _ := range ap.lcNameFromASG {
				for _, value := range page.LaunchConfigurations {
					if ap.lcNameFromASG[i] == *value.LaunchConfigurationName {
						ap.amiIDLCFromASG = append(ap.amiIDLCFromASG, *value.ImageId)
					}
				}
			}
			pageNum++
			return pageNum <= LCPAGE
		})
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

/* LaunchConfigのリストと作成日時を取得 */
func (ap *ASGParams) FetchListLCNameCreatedTime() {

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
			return pageNum <= LCPAGE
		})

	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

/* LaunchConfig名を取得 */
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

/* SSMパラメータからSourceAMI-IDを取得 */
func (sp *SSMParams) GetAMIFromSSM() {
	for i, _ := range sp.key {
		sp.amiIdSSM = append(sp.amiIdSSM, sp.FetchSSMParamsValue(&sp.key[i]))
	}
}

/* SSMパラメータから `_source_ami` に該当するkeyを取得 */
func (sp *SSMParams) FetchSSMParamsKey() {
	pageNum := 0
	params := &ssm.DescribeParametersInput{}
	err := svcSSM.DescribeParametersPages(params,
		func(page *ssm.DescribeParametersOutput, lastPage bool) bool {
			for _, value := range page.Parameters {
				if strings.Contains(*value.Name, SSMSEARCHWORD) {
					sp.key = append(sp.key, *value.Name)
				}
			}
			pageNum++
			return pageNum <= SSMPAGE
		})
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

/* SSMパラメータで指定したKeyのValueを返す */
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

	return sp.value
}

/* AccountAliasを取得 */
func (s *SlackParams) GetAccountAlias() {
	params := &iam.ListAccountAliasesInput{}
	res, err := svcIAM.ListAccountAliases(params)
	if err != nil {
		fmt.Println("Got error fetching parameter: ", err)
		fmt.Println(err.Error())
		os.Exit(1)
	}
	if res.AccountAliases == nil{
		s.accountAlias = "None"
	}else{
		s.accountAlias = *res.AccountAliases[0]
	}
}

/* 結果をSlackに投稿 */
func (s *SlackParams) PostSlack(accountName string, beforeExistAMIs int, deleteAMIs int, deleteLCs int, excludedListSSM string, excludedListLC string) {
	field1 := slack.Field{Title: "Account", Value: accountName}
	field2 := slack.Field{Title: "Before Total Exist AMIs", Value: strconv.Itoa(beforeExistAMIs)}
	field3 := slack.Field{Title: "Total Delete AMIs and Snapshots", Value: strconv.Itoa(deleteLCs)}
	field4 := slack.Field{Title: "Total Delete LauchConfigs", Value: strconv.Itoa(deleteAMIs)}
	field5 := slack.Field{Title: "Excluded AMIs ( SSM )", Value: excludedListSSM}
	field6 := slack.Field{Title: "Excluded AMIs ( LC )", Value: excludedListLC}

	attachment := slack.Attachment{}
	attachment.AddField(field1).AddField(field2).AddField(field3).AddField(field4).AddField(field5).AddField(field6)
	color := "warning"
	attachment.Color = &color
	payload := slack.Payload{
		Username:    s.username,
		Channel:     s.channel,
		Attachments: []slack.Attachment{attachment},
	}
	err := slack.Send(WEBHOOKURL, "", payload)
	if len(err) > 0 {
		os.Exit(1)
	}
}

package quayconfig

import (
	"encoding/json"
	"fmt"
	"repliquay/repliquay/internal/apicall"
)

type QuayConfig struct {
	Debug, SkipVerify, DryRun, Insecure bool
	SleepPeriod, Retries                int
}

type QuayOrgResponse struct {
	Organizations []QuayOrgStruct
}

type QuayOrgStruct struct {
	Name                string
	Avatar              AvatarStruct
	Can_create_repo     bool
	Public              bool
	Is_org_admin        bool
	Preferred_namespace bool
}

type AvatarStruct struct {
	Name  string
	Hash  string
	Color string
	Kind  string
}

type TeamsStruct struct {
	Name         string
	Description  string
	Role         string
	Avatar       AvatarStruct
	Can_view     bool
	Repo_count   int
	Member_count int
	Is_synced    bool
}

type QuayOrgApiResponse struct {
	Name                  string
	Email                 string
	Avatar                AvatarStruct
	Is_admin              bool
	Is_member             bool
	Teams                 map[string]TeamsStruct
	Ordered_teams         []string
	Invoice_email         bool
	Invoice_email_address string
	Tag_expiration_s      int
	Is_free_account       bool
}

type QuayRepositories struct {
	Repositories []QuayRepository
}

type QuayRepository struct {
	Namespace     string
	Name          string
	Description   string
	Is_public     bool
	Kind          string
	State         string
	Last_modified string
	Popularity    float32
	Is_starred    bool
}

type QuayRobotsApi struct {
	Robots []QuayRobotApi
}

type QuayRobotApi struct {
	Name          string
	Created       string
	Last_accessed string
	Teams         []RobotTeamStruct
	Repositories  []string
	Description   string
}

type RobotTeamStruct struct {
	Name   string
	Avatar AvatarStruct
}

type QuayTeamMember struct {
	Name    string
	Members []TeamMembers
	CanEdit bool
}

type TeamMembers struct {
	Name     string
	Kind     string
	Is_robot bool
	Avatar   AvatarStruct
	Invited  bool
}

func (qc *QuayConfig) SetGlobalVars(debug bool, skipverify bool, dryrun bool, insecure bool, sleepPeriod int, retries int) {
	qc.Debug = debug
	qc.SkipVerify = skipverify
	qc.DryRun = dryrun
	qc.Insecure = insecure
	qc.SleepPeriod = sleepPeriod
	qc.Retries = retries
}

func (qc *QuayConfig) GetConfFromQuay(quay string, token string, max_conn int) (orgs []string) {
	hostConn := apicall.HostConnection{Max_connections: max_conn, Hostname: quay, QueueLength: 0}
	hostConn.SetGlobalVars(qc.Debug, qc.SkipVerify, qc.DryRun, qc.Insecure, qc.SleepPeriod, qc.Retries)
	httpCode, orgList := hostConn.ApiCall(quay, "/api/v1/user/", "GET", token, "", "get user organizations", &qc.Retries)
	var quay_orgs QuayOrgResponse

	json.Unmarshal([]byte(orgList), &quay_orgs)

	if len(orgList) > 0 {
		for i := 0; i < len(quay_orgs.Organizations); i++ {
			orgs = append(orgs, quay_orgs.Organizations[i].Name)
			fmt.Println("orgs-----", httpCode, "orgList-----", quay_orgs.Organizations[i].Name)
			getQuayOrg(quay, token, quay_orgs.Organizations[i].Name, &hostConn)
			getQuayOrgRobots(quay, token, quay_orgs.Organizations[i].Name, &hostConn)
			getQuayRepos(quay, token, quay_orgs.Organizations[i].Name, &hostConn)
		}
	}
	return orgs
}

func getQuayOrg(quay string, token string, orgName string, hostStatus *apicall.HostConnection) {
	var quay_org QuayOrgApiResponse
	fmt.Println("getQuayOrg------: /api/v1/organization/" + orgName)
	_, apiResponse := hostStatus.ApiCall(quay, "/api/v1/organization/"+orgName, "GET", token, "", "get organizations details", &hostStatus.Retries)
	// fmt.Println("httpcode", httpCode, "org", orgName, "apiResponse", apiResponse)
	json.Unmarshal([]byte(apiResponse), &quay_org)
	fmt.Println("quay_org parsed name", quay_org.Name, "teams ", quay_org.Teams, "ordered teams", quay_org.Ordered_teams, "apiResponse", apiResponse)
	if len(quay_org.Ordered_teams) > 0 {
		for j := range quay_org.Ordered_teams {
			getQuayOrgTeams(quay, token, orgName, quay_org.Ordered_teams[j], hostStatus)
		}
	}
}

func getQuayOrgRobots(quay string, token string, orgName string, hostStatus *apicall.HostConnection) {
	var quay_org_robots QuayRobotsApi
	// /api/v1/organization/organization2/robots?permissions=true&token=false
	fmt.Println("getQuayOrgRobots------: /api/v1/organization/" + orgName + "/robots?permission=true&token=false")
	_, apiResponse := hostStatus.ApiCall(quay, "/api/v1/organization/"+orgName+"/robots?permission=true&token=false", "GET", token, "", "get organizations robots", &hostStatus.Retries)
	json.Unmarshal([]byte(apiResponse), &quay_org_robots)
	if len(quay_org_robots.Robots) > 0 {
		fmt.Println("quay_org_robots parsed len", len(quay_org_robots.Robots), "name ", quay_org_robots.Robots[0].Name, "apiResponse", apiResponse)
	}
}

func getQuayOrgTeams(quay string, token string, orgName string, team string, hostStatus *apicall.HostConnection) {
	var quay_org_team_members QuayTeamMember
	// /api/v1/organization/organization2/team/devteam/members
	fmt.Println("getQuayOrgTeams------: /api/v1/organization/" + orgName + "/team/" + team + "/members")
	_, apiResponse := hostStatus.ApiCall(quay, "/api/v1/organization/"+orgName+"/robots?permission=true&token=false", "GET", token, "", "get organizations robots", &hostStatus.Retries)
	json.Unmarshal([]byte(apiResponse), &quay_org_team_members)
	if len(quay_org_team_members.Members) > 0 {
		fmt.Println("quay_org_team_members parsed len", len(quay_org_team_members.Members), "name ", quay_org_team_members, "apiResponse", apiResponse)
	}
}

func getQuayRepos(quay string, token string, orgName string, hostStatus *apicall.HostConnection) {
	var quay_repos QuayRepositories
	fmt.Println("getQuayRepos------: /api/v1/repository")
	httpCode, apiResponse := hostStatus.ApiCall(quay, "/api/v1/repository?public=true&namespace="+orgName, "GET", token, "", "get organizations repositories", &hostStatus.Retries)
	fmt.Println("httpcode", httpCode, "apiResponse", apiResponse)
	json.Unmarshal([]byte(apiResponse), &quay_repos)
	fmt.Println("repositories parsed len", len(quay_repos.Repositories), "repo[0] name ", quay_repos.Repositories[0].Name, "quay_repos array", quay_repos.Repositories, "apiResponse", apiResponse)
}

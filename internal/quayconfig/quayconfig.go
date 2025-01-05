package quayconfig

import (
	"encoding/json"
	"fmt"
	"repliquay/repliquay/internal/apicall"
	"strings"
	"sync"
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

type QuayRepoPerms struct {
	Permissions map[string]Repo_perm
}

type Repo_perm struct {
	Role          string
	Name          string
	Is_robot      bool
	Avatar        AvatarStruct
	Is_org_member bool
}

func (qc *QuayConfig) SetGlobalVars(debug bool, skipverify bool, dryrun bool, insecure bool, sleepPeriod int, retries int) {
	qc.Debug = debug
	qc.SkipVerify = skipverify
	qc.DryRun = dryrun
	qc.Insecure = insecure
	qc.SleepPeriod = sleepPeriod
	qc.Retries = retries
}

type robotStruct struct {
	Name        string
	Description string
}

type teamStruct struct {
	Name        string
	Description string
	Role        string
}

func (qc *QuayConfig) GetConfFromQuay(quay string, token string, max_conn int) (org_repos map[string][]string, org_teams map[string][]teamStruct, org_robots map[string][]robotStruct, repo_perms map[string]map[string][]string) {
	hostConn := apicall.HostConnection{Max_connections: max_conn, Hostname: quay, QueueLength: 0}
	hostConn.SetGlobalVars(qc.Debug, qc.SkipVerify, qc.DryRun, qc.Insecure, qc.SleepPeriod, qc.Retries)
	_, orgList := hostConn.ApiCall(quay, "/api/v1/user/", "GET", token, "", "get user organizations", &qc.Retries)
	var quay_orgs QuayOrgResponse

	org_repos = make(map[string][]string)
	org_teams = make(map[string][]teamStruct)
	org_robots = make(map[string][]robotStruct)
	repo_perms = make(map[string]map[string][]string)
	var wg sync.WaitGroup

	json.Unmarshal([]byte(orgList), &quay_orgs)

	if len(quay_orgs.Organizations) > 0 {
		for i, v := range quay_orgs.Organizations {
			wg.Add(1)
			go func() {
				defer wg.Done()
				org_teams[v.Name] = getQuayOrg(quay, token, quay_orgs.Organizations[i].Name, &hostConn)
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				org_robots[v.Name] = getQuayOrgRobots(quay, token, quay_orgs.Organizations[i].Name, &hostConn)
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				org_repos[v.Name], repo_perms[v.Name] = getQuayRepos(quay, token, quay_orgs.Organizations[i].Name, &hostConn)
			}()
			if qc.Debug {
				for _, k := range org_teams[v.Name] {
					fmt.Printf("org %s team %s\n", v.Name, k)
				}
				for _, k := range org_robots[v.Name] {
					fmt.Printf("org %s robot %s\n", v.Name, k)
				}
				for _, k := range org_repos[v.Name] {
					fmt.Printf("org %s repo %s\n", v.Name, k)
				}
				for _, k := range repo_perms[v.Name] {
					fmt.Printf("org %s perm %s\n", v.Name, k)
				}
			}
			wg.Wait()
		}
	}
	return
}

func getQuayOrg(quay string, token string, orgName string, hostStatus *apicall.HostConnection) (team_list []teamStruct) {
	var quay_org QuayOrgApiResponse
	fmt.Printf("Get Quay organization %s\n", orgName)
	_, apiResponse := hostStatus.ApiCall(quay, "/api/v1/organization/"+orgName, "GET", token, "", "get "+orgName+" organization details", &hostStatus.Retries)
	// fmt.Println("httpcode", httpCode, "org", orgName, "apiResponse", apiResponse)
	json.Unmarshal([]byte(apiResponse), &quay_org)
	fmt.Printf("Get Quay organization %s...\tDone\n", orgName)
	for _, v := range quay_org.Ordered_teams {
		if !quay_org.Teams[v].Is_synced {
			team_list = append(team_list, teamStruct{Name: quay_org.Teams[v].Name, Description: quay_org.Teams[v].Description, Role: quay_org.Teams[v].Role})
		}
	}
	return
}

func getQuayOrgRobots(quay string, token string, orgName string, hostStatus *apicall.HostConnection) (robots_list []robotStruct) {
	var quay_org_robots QuayRobotsApi
	// /api/v1/organization/organization2/robots?permissions=true&token=false
	_, apiResponse := hostStatus.ApiCall(quay, "/api/v1/organization/"+orgName+"/robots?permission=true&token=false", "GET", token, "", "get "+orgName+" organization robots", &hostStatus.Retries)
	json.Unmarshal([]byte(apiResponse), &quay_org_robots)
	if len(quay_org_robots.Robots) > 0 {
		for _, v := range quay_org_robots.Robots {
			rb := strings.Split(v.Name, "+")
			robots_list = append(robots_list, robotStruct{Name: rb[1], Description: v.Description})
		}
	}
	return
}

// func getQuayOrgTeamMembers(quay string, token string, orgName string, team string, hostStatus *apicall.HostConnection) {
// 	var quay_org_team_members QuayTeamMember
// 	// /api/v1/organization/organization2/team/devteam/members
// 	fmt.Println("getQuayOrgTeams------: /api/v1/organization/" + orgName + "/team/" + team + "/members")
// 	_, apiResponse := hostStatus.ApiCall(quay, "/api/v1/organization/"+orgName+"/robots?permission=true&token=false", "GET", token, "", "get organizations robots", &hostStatus.Retries)
// 	json.Unmarshal([]byte(apiResponse), &quay_org_team_members)
// 	if len(quay_org_team_members.Members) > 0 {
// 		fmt.Println("quay_org_team_members parsed len", len(quay_org_team_members.Members), "name ", quay_org_team_members, "apiResponse", apiResponse)
// 	}
// }

func getQuayRepos(quay string, token string, orgName string, hostStatus *apicall.HostConnection) (org_repos []string, org_repo_perms map[string][]string) {
	var quay_repos QuayRepositories
	org_repo_perms = make(map[string][]string)
	var wg sync.WaitGroup

	fmt.Printf("Get Quay repositories for org %s\n", orgName)
	_, apiResponse := hostStatus.ApiCall(quay, "/api/v1/repository?public=true&namespace="+orgName, "GET", token, "", "get "+orgName+" organization repositories", &hostStatus.Retries)
	// fmt.Println("httpcode", httpCode, "apiResponse", apiResponse)
	json.Unmarshal([]byte(apiResponse), &quay_repos)
	fmt.Printf("Get Quay repositories for org %s...\tDone\n", orgName)

	for _, v := range quay_repos.Repositories {
		org_repos = append(org_repos, v.Name)
		wg.Add(1)
		go func() {
			defer wg.Done()
			org_repo_perms[v.Name] = getQuayRepoPerms(quay, token, orgName, v.Name, hostStatus)
		}()
	}
	wg.Wait()
	return
}

func getQuayRepoPerms(quay string, token string, orgName string, repo_name string, hostStatus *apicall.HostConnection) (repo_perms []string) {
	// repo_perms repo_name#kind{team/robot}#name#role
	var quay_repo_perms QuayRepoPerms
	fmt.Printf("Get Quay %s/%s repository team permissions\n", orgName, repo_name)
	_, apiResponse := hostStatus.ApiCall(quay, "/api/v1/repository/"+orgName+"/"+repo_name+"/permissions/team/", "GET", token, "", "get "+orgName+" organization repository "+repo_name+" team permission", &hostStatus.Retries)
	// fmt.Println("httpcode", httpCode, "apiResponse", apiResponse)
	json.Unmarshal([]byte(apiResponse), &quay_repo_perms)
	fmt.Printf("Get Quay %s/%s repository team permissions...\tDone\n", orgName, repo_name)
	// fmt.Println("repository permission parsed len", len(quay_repo_perms.Permissions), "apiResponse", apiResponse)
	for _, v := range quay_repo_perms.Permissions {
		repo_perms = append(repo_perms, "team#"+v.Name+"#"+v.Role)
	}
	fmt.Printf("Get Quay %s/%s repository user permissions\n", orgName, repo_name)
	_, apiResponse = hostStatus.ApiCall(quay, "/api/v1/repository/"+orgName+"/"+repo_name+"/permissions/user/", "GET", token, "", "get "+orgName+" organization repository "+repo_name+" user permission", &hostStatus.Retries)
	// fmt.Println("httpcode", httpCode, "apiResponse", apiResponse)
	json.Unmarshal([]byte(apiResponse), &quay_repo_perms)
	fmt.Printf("Get Quay %s/%s repository user permissions...\tDone\n", orgName, repo_name)
	// fmt.Println("repository permission parsed len", len(quay_repo_perms.Permissions), "apiResponse", apiResponse)
	for _, v := range quay_repo_perms.Permissions {
		if v.Is_robot {
			rb := strings.Split(v.Name, "+")
			repo_perms = append(repo_perms, "robot#"+rb[1]+"#"+v.Role)
		}
	}
	return
}

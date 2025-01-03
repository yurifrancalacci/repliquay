package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"repliquay/repliquay/internal/apicall"
	"repliquay/repliquay/internal/quayconfig"
	"slices"
	"strings"
	"sync"
	"time"

	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

type Quays struct {
	HostToken []HostToken `yaml:"quays"`
}

type HostToken struct {
	Host          string `yaml:"host"`
	Token         string `yaml:"token"`
	MaxConnection int    `yaml:"max_connections"`
}

type Organization struct {
	Name        string        `yaml:"quay_organization"`
	OrgRoleName string        `yaml:"quay_organization_role_name"`
	RepoList    []RepoStruct  `yaml:"repositories"`
	RobotList   []RobotStruct `yaml:"robots"`
	TeamsList   []TeamStruct  `yaml:"teams"`
}

type RepoStruct struct {
	Name           string               `yaml:"name"`
	Mirror         bool                 `yaml:"mirror"`
	PermissionList RepoPermissionStruct `yaml:"permissions"`
}

type RobotStruct struct {
	Name        string `yaml:"name"`
	Description string `yaml:"desc"`
}

type TeamStruct struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	GroupDN     string `yaml:"group_dn"`
	Role        string `yaml:"role"`
}

type RepoPermissionStruct struct {
	Robots []PermStruct `yaml:"robots"`
	Teams  []PermStruct `yaml:"teams"`
}

type PermStruct struct {
	Name           string `yaml:"name"`
	Role           string `yaml:"role"`
	PermissionKind string
	RepoName       string
	Organization   string
}

// vars
var (
	insecure    bool
	ldapSync    bool
	dryRun      bool
	sleepPeriod int
	debug       bool
	retries     int
	skipVerify  bool
	clone       bool
)

func checkLogin(quayHost string, token string, hostConn *apicall.HostConnection) (login_ok bool) {
	fmt.Println("check login")
	retryCounter := 0

	hostConn.ApiCall(quayHost, "/api/v1/user/logs", "GET", token, "", "checking Logins", &retryCounter)
	login_ok = true
	return
}

func createOrg(quayHost string, orgList Organization, token string, hostConn *apicall.HostConnection) (status bool) {
	retryCounter := 0

	if debug {
		fmt.Println("Creating Org...", orgList.Name)
	}
	hostConn.ApiCall(
		quayHost,
		"/api/v1/organization/",
		"POST",
		token,
		`{"name":"`+orgList.Name+`"}`,
		"create organization"+orgList.Name,
		&retryCounter,
	)
	status = true
	return
}

func createRepo(quayHost string, orgName string, repoConfig []RepoStruct, token string, hostConn *apicall.HostConnection) (status bool) {
	var wg sync.WaitGroup
	retryCounter := 0

	if debug {
		fmt.Printf("Creating %d repos\n", len(repoConfig))
	}
	for i, v := range repoConfig {
		if debug {
			fmt.Printf("Creating Repo %d: %s\n", i, v.Name)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			hostConn.ApiCall(
				quayHost,
				"/api/v1/repository",
				"POST",
				token,
				`{"repository":"`+v.Name+`","visibility":"private","namespace":"`+orgName+`","description":"repository description"}`,
				"create repository "+v.Name,
				&retryCounter,
			)
		}()
	}
	wg.Wait()
	status = true
	return
}

func createRepoPermission(quayHost string, permList []PermStruct, token string, hostConn *apicall.HostConnection) (status bool) {
	var wg sync.WaitGroup
	retryCounter := 0

	if debug {
		fmt.Printf("Creating %d permissions for host %s\n", len(permList), quayHost)
	}

	for _, v := range permList {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v.PermissionKind == "robots" {
				hostConn.ApiCall(
					quayHost,
					"/api/v1/repository/"+v.Organization+"/"+v.RepoName+"/permissions/user/"+v.Organization+"+"+v.Name,
					"PUT",
					token,
					`{"role":"`+v.Role+`"}`,
					"create repo permission for robot "+v.Name+" and role "+v.Role,
					&retryCounter,
				)
			} else {
				hostConn.ApiCall(
					quayHost,
					"/api/v1/repository/"+v.Organization+"/"+v.RepoName+"/permissions/team/"+v.Name,
					"PUT",
					token,
					`{"role":"`+v.Role+`"}`,
					"create repo "+v.Name+" permission for teams "+v.Name+" and role "+v.Role,
					&retryCounter,
				)
			}
		}()
	}
	wg.Wait()
	status = true
	return
}

func createPermissionList(repoConfig []RepoStruct, permissionName string, orgName string) (permList []PermStruct) {
	if debug {
		fmt.Printf("Mapping Permission for %d repos for %s\n", len(repoConfig), permissionName)
	}

	totalPermission := 0
	for _, v := range repoConfig {

		var loopList []PermStruct
		if permissionName == "robots" {
			loopList = v.PermissionList.Robots
		} else {
			loopList = v.PermissionList.Teams
		}
		if debug {
			fmt.Printf("Mapping permission for repo: %s kind %s\n", v.Name, permissionName)
		}
		for _, vv := range loopList {
			totalPermission++
			permList = append(permList, PermStruct{Name: vv.Name, Role: vv.Role, PermissionKind: permissionName, RepoName: v.Name, Organization: orgName})
		}
	}
	fmt.Printf("Total mapped %s permission for org: %d\n", permissionName, totalPermission)
	return
}

func createRobotTeam(quayHost string, orgName string, robotList []RobotStruct, teamList []TeamStruct, token string, hostConn *apicall.HostConnection) (status bool) {
	var wg sync.WaitGroup
	retryCounter := 0
	// robot
	if debug {
		fmt.Println("creating ", len(robotList), "robots for", orgName, "host", quayHost)
	}
	for _, v := range robotList {
		if debug {
			fmt.Println("Creating robot", v)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			hostConn.ApiCall(quayHost, "/api/v1/organization/"+orgName+"/robots/"+v.Name, "PUT", token, `{"description":"`+v.Description+`"}`, "create robot "+v.Name, &retryCounter)
		}()
	}
	// teams
	if debug {
		fmt.Println("creating ", len(teamList), "teams")
	}
	for _, v := range teamList {
		if debug {
			fmt.Println("Creating team", v)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			hostConn.ApiCall(quayHost, "/api/v1/organization/"+orgName+"/team/"+v.Name, "PUT", token, `{"role":"`+v.Role+`"}`, "create team "+v.Name, &retryCounter)
			if ldapSync {
				hostConn.ApiCall(quayHost, "/api/v1/organization/"+orgName+"/team/"+v.Name+"/syncing", "POST", token, `{"group_dn":"`+v.GroupDN+`"}`, "create team sync "+v.Name, &retryCounter)
			}
		}()
	}
	wg.Wait()
	status = true
	return
}

func parseIniFile(inifile string, quay string, repolist []string, sleep int, insec bool, ldap bool, dry bool, _clone bool) (quaysfile string, repo []string, sleepPeriod int, insecure bool, ldapSync bool, dryRun bool, clone bool) {
	inidata, err := ini.Load(inifile)

	if err != nil {
		log.Print("Warning: no conf option found, loading default values from /repos/repliquay.conf")
		quaysfile, repo, sleepPeriod, insecure, ldapSync, dryRun, clone = quay, repolist, sleep, insec, ldap, dry, _clone
		return
	}
	quaysfile = inidata.Section("quays").Key("file").String()
	repo = inidata.Section("repos").Key("files").Strings(",")
	sleepPeriod, _ = inidata.Section("params").Key("sleep").Int()
	debug, _ = inidata.Section("params").Key("debug").Bool()
	ldapSync, _ = inidata.Section("params").Key("ldapsync").Bool()
	dryRun, _ = inidata.Section("params").Key("dryrun").Bool()
	clone, _ = inidata.Section("params").Key("clone").Bool()
	return
}

func main() {
	var quays Quays
	var org Organization
	var parsedOrg []Organization
	var permList []PermStruct
	hostConn := make(map[string]*apicall.HostConnection)

	var (
		quaysfile string
		repo      []string
		confFile  string
	)

	t1 := time.Now()
	flag.Func("repo", "quay repo file name", func(s string) error {
		_, err := os.Stat(s)
		if err == nil {
			repo = append(repo, s)
			return nil
		} else {
			return errors.New("file does not exists")
		}
	})
	flag.StringVar(&quaysfile, "quaysfile", "", "quay token file name")
	flag.StringVar(&confFile, "conf", "/repos/repliquay.conf", "repliquay config file (override all opts)")
	flag.IntVar(&sleepPeriod, "sleep", 100, "sleep length ms when reaching max connection")
	flag.IntVar(&retries, "retries", 3, "max retries on api call failure")
	flag.BoolVar(&debug, "debug", false, "print debug messages (default false)")
	flag.BoolVar(&insecure, "insecure", false, "disable TLS connection (default false)")
	flag.BoolVar(&ldapSync, "ldapsync", false, "enable ldap sync (default false)")
	flag.BoolVar(&dryRun, "dryrun", false, "enable dry run (default false)")
	flag.BoolVar(&skipVerify, "skipVerify", false, "enable/disable TLS validation")
	flag.BoolVar(&clone, "clone", false, "clone first quay configuration to others. Requires >= 2 quays (ignore all other options)")

	flag.Parse()

	p, _ := os.Executable()
	_, err := os.Stat(p + "/" + confFile)
	if err != nil {
		quaysfile, repo, sleepPeriod, insecure, ldapSync, dryRun, clone = parseIniFile(confFile, quaysfile, repo, sleepPeriod, insecure, ldapSync, dryRun, clone)
	} else {
		fmt.Println("No config file provided ")
	}

	if debug {
		for _, v := range repo {
			fmt.Printf("- Name: %s\n", v)
		}
		fmt.Printf("quayfile %s\n\ninsecure %t\n", quaysfile, insecure)
	}

	yamlData, err := os.ReadFile(quaysfile)

	if err != nil {
		log.Fatal("Error while reading quays file ", err)
	}

	yaml.Unmarshal(yamlData, &quays)
	var orgList []string
	if !clone {	
		for _, r := range repo {
			yamlData, err = os.ReadFile(r)
			if err != nil {
				log.Fatal("Error while reading quays file ", err)
			}
			yaml.Unmarshal(yamlData, &org)
			if slices.Contains(orgList, org.Name) {
				log.Fatalf("Duplicated organization %s", org.Name)
			} else {
				parsedOrg = append(parsedOrg, org)
				orgList = append(orgList, org.Name)
			}
		}
	}
	if clone {
		var qc quayconfig.QuayConfig
		parsedOrg = nil
		orgList = nil
		qc.SetGlobalVars(debug, skipVerify, dryRun, insecure, sleepPeriod, retries)
		if len(quays.HostToken) < 2 {
			log.Fatalf("Cannot clone. 2 quays registry required, got %d", len(quays.HostToken))
		}
		log.Printf("Cloning repository %s to %s", quays.HostToken[0].Host, quays.HostToken[1].Host)
		org_repos, org_teams, org_robots, org_repo_perms := qc.GetConfFromQuay(quays.HostToken[0].Host, quays.HostToken[0].Token, quays.HostToken[0].MaxConnection)

		//remove first quay instance as cloning from first to others
		_, tempQuay := quays.HostToken[0], quays.HostToken[1:]
		quays.HostToken = tempQuay

		for k, r := range org_repos {
			var robotList []RobotStruct
			var teamList []TeamStruct
			var repoList []RepoStruct
			for _, v := range org_robots[k] {
				robotList = append(robotList, RobotStruct{Name: v.Name, Description: v.Description})
			}
			for _, v := range org_teams[k] {
				teamList = append(teamList, TeamStruct{Name: v.Name, Description: v.Description, GroupDN: "", Role: v.Role})
			}

			for _, v := range r {
				var robotPerms, teamsPerms []PermStruct
				for _, p := range org_repo_perms[k][v] {
					// var kind, n, rp string
					perm := strings.Split(p, "#")
					kind, n, rp := perm[0], perm[1], perm[2]
					if kind == "robot" {
						robotPerms = append(robotPerms, PermStruct{Name: n, Role: rp, PermissionKind: "robots", RepoName: v, Organization: k})
					} else {
						teamsPerms = append(teamsPerms, PermStruct{Name: n, Role: rp, PermissionKind: "teams", RepoName: v, Organization: k})
					}
				}
				repoList = append(repoList, RepoStruct{Mirror: false, Name: v, PermissionList: RepoPermissionStruct{Robots: robotPerms, Teams: teamsPerms}})
			}
			parsedOrg = append(parsedOrg, Organization{Name: k, OrgRoleName: k, RobotList: robotList, TeamsList: teamList, RepoList: repoList})
		}

		// os.Exit(0)
	}

	fmt.Printf("Repliquay: repliquayting... be patient\n")

	var wg sync.WaitGroup
	for _, o := range parsedOrg {
		// fmt.Printf("Parsed ORG------%s\n", o.Name)
		permList = slices.Concat(permList, createPermissionList(o.RepoList, "robots", o.Name), createPermissionList(o.RepoList, "teams", o.Name))
	}

	for _, v := range quays.HostToken {
		h := apicall.HostConnection{QueueLength: 0, Max_connections: v.MaxConnection, Hostname: v.Host}
		h.SetGlobalVars(debug, skipVerify, dryRun, insecure, sleepPeriod, retries)
		hostConn[v.Host] = &h
		wg.Add(1)
		go func() {
			defer wg.Done()
			if !dryRun {
				if !checkLogin(v.Host, v.Token, hostConn[v.Host]) {
					log.Fatal("Error logging to quay hosts")
				}
			}
		}()
	}
	wg.Wait()

	for _, v := range quays.HostToken {
		fmt.Println("len parsedOrg", len(parsedOrg))
		for i, o := range parsedOrg {
			wg.Add(1)
			go func() {
				defer wg.Done()
				fmt.Printf("creating organization - Host: %s\t- %s\n", v.Host, o.Name)
				createOrg(v.Host, o, v.Token, hostConn[v.Host])
				fmt.Printf("creating robots and teams for organization %s - Host: %s\n", o.Name, v.Host)
				createRobotTeam(v.Host, o.Name, o.RobotList, o.TeamsList, v.Token, hostConn[v.Host])
				fmt.Printf("creating repositories for organization %s - Host: %s\n", o.Name, v.Host)
				createRepo(v.Host, o.Name, o.RepoList, v.Token, hostConn[v.Host])
				if i == 0 {
					fmt.Printf("creating permissions for repositories in organization %s - Host: %s\n", o.Name, v.Host)
					createRepoPermission(v.Host, permList, v.Token, hostConn[v.Host])
				}
			}()
		}
	}
	wg.Wait()
	fmt.Printf("Repliquay: mission completed in %s\n", time.Since(t1))
}

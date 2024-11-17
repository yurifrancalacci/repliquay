package main

import (
	"fmt"
	"log"
	"os"
	"io"
	"sync"
	"bytes"
	"time"
	"slices"
	"flag"
	"errors"
	// "time"
	"net/http"
	"gopkg.in/yaml.v3"
)

type Quays struct {
	HostToken []HostToken `yaml:"quays"`
   }

type HostToken struct {
	Host string `yaml:"host"`
	Token string `yaml:"token"`
	// MaxConnection int `yaml:"max_connections"`
	// QueueLength int
   }

type Organization struct {
	Name string `yaml:"quay_organization"`
	OrgRoleName string `yaml:"quay_organization_role_name"`
	RepoList []RepoStruct `yaml:"repositories"`
	RobotList []RobotStruct `yaml:"robots"`
	TeamsList []TeamStruct `yaml:"teams"`
   }

type RepoStruct struct {
	Name string `yaml:"name"`
	Mirror bool `yaml:"mirror"`
	PermissionList RepoPermissionStruct `yaml:"permissions"`
}

type RobotStruct struct {
	Name string `yaml:"name"`
	Description string `yaml:"desc"`
}

type TeamStruct struct {
	Name string `yaml:"name"`
	Description string `yaml:"description"`
	GroupDN string `yaml:"group_dn"`
	Role string `yaml:"role"`
}

type RepoPermissionStruct struct {
	Robots []PermStruct `yaml:"robots"`
	Teams []PermStruct `yaml:"teams"`
}

type PermStruct struct {
	Name string `yaml:"name"`
	Role string `yaml:"role"`
	PermissionKind string
	RepoName string
}

// vars
var(
	parallel int
	insecure bool
)
// func
func apiCall(host string, url string, method string, token string, bodyData string, action string) (httpCode int) {

	var enableTLS string

	client := &http.Client{}
	req := &http.Request{}
	if !insecure {
		enableTLS = "s"
	}

	if bodyData != "" {
		jsonBody := []byte(bodyData)
		bodyReader := bytes.NewReader(jsonBody)
		req, _ = http.NewRequest(method, "http"+ enableTLS +"://" + host + url, bodyReader)
	} else {
		req, _ = http.NewRequest(method, "http"+ enableTLS +"://" + host + url, nil)
	}

	if token != "" {
		req.Header.Add("Authorization", "Bearer " + token)
		req.Header.Add("Content-Type", "application/json")
	}
	
	res, err := client.Do(req)
	
	if err != nil {
		log.Fatal(err)
	}
	res_body, err := io.ReadAll(res.Body)
	res.Body.Close()

	if res.StatusCode == 401 {
		log.Fatalf("%s Unauthorized Response failed with status code: %d and\nbody: %s\nRequest data %s\nurl %s method %s", host, res.StatusCode, res_body, bodyData, url, method)
	} 

	if res.StatusCode > 499 {
		log.Fatalf("%s Response failed with status code: %d and\nbody: %s\nRequest data %s\nurl %s method %s", host, res.StatusCode, res_body, bodyData, url, method)
	} else if res.StatusCode > 299 {
		log.Printf("%s Response failed with status code: %d and\nbody: %s\nRequest data %surl %s method %s", host, res.StatusCode, res_body, bodyData, url, method)
	} else {
		log.Printf("%s Action %s completed\n", host, action)
	}
	if err != nil {
		log.Fatal(err)
	}
	// fmt.Printf("%s", body)
	httpCode = res.StatusCode
	return
}

func checkLogin(quays Quays) (login_ok bool) {
	var wg sync.WaitGroup
	fmt.Println("check login")

	queueLength := 0
	for _, v := range quays.HostToken {
		queueLength++

		for queueLength > parallel {
			fmt.Println("sleep 1")
			time.Sleep(1 * time.Second)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			apiCall(v.Host, "/api/v1/user/logs", "GET", v.Token, "", "checking Logins")
		}(&queueLength)
	}
	wg.Wait()
	login_ok = true
	return
}

func createOrg(quayHost string, orgList Organization, token string) (status bool) {
	fmt.Println("Creating Org...", orgList.Name)
	apiCall(
		quayHost,
		"/api/v1/organization/",
		"POST",
		token,
		`{"name":"`+ orgList.Name +`"}`,
		"create organization" + orgList.Name,
	)
	status = true
	return
}

func createRepo(quayHost string, orgName string, repoConfig []RepoStruct, token string) (status bool) {
	var wg sync.WaitGroup
	queueLength := 0
	fmt.Printf("Creating %d repos\n", len(repoConfig))
	fmt.Printf("Parallel value %d\n", parallel)

	for i, v := range repoConfig {
		fmt.Printf("Creating Repo %d: %s\n", i, v.Name)
		queueLength++
		for queueLength > parallel {
			fmt.Println("sleep 1")
			time.Sleep(1 * time.Second)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			apiCall(
				quayHost,
				"/api/v1/repository",
				"POST",
				token,
				`{"repository":"`+ v.Name +`","visibility":"private","namespace":"`+ orgName +`","description":"repository description"}`,
				"create repository "+ v.Name,
			)
		}(&queueLength)
	}
	wg.Wait()
	status = true
	return
}

func createRepoPermission(quayHost string, orgName string, permList []PermStruct, token string) (status bool) {
	var wg sync.WaitGroup
	queueLength := 0
	fmt.Printf("Creating %d permissions\n", len(permList))

	for _, v := range permList {
		queueLength++

		for queueLength > parallel {
			fmt.Println("sleep 5 seconds...")
			time.Sleep(5000 * time.Millisecond)
			// 
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			if v.PermissionKind == "robots" {
				apiCall(
					quayHost,
					"/api/v1/repository/"+ orgName +"/"+v.RepoName +"/permissions/user/"+ orgName + "+" + v.Name ,
					"PUT",
					token,
					`{"role":"`+ v.Role +`"}`,
					"create repo permission for robot "+ v.Name + " and role "+ v.Role,
				)
			} else {
				apiCall(
					quayHost,
					"/api/v1/repository/"+ orgName +"/"+v.RepoName +"/permissions/team/"+ v.Name ,
					"PUT",
					token,
					`{"role":"`+ v.Role +`"}`,
					"create repo "+v.Name+" permission for teams "+ v.Name + " and role "+ v.Role,
				)
			}
		}(&queueLength)
	}
	wg.Wait()
	status = true
	return
}

func createPermissionList(repoConfig []RepoStruct, permissionName string) (permList []PermStruct) {
	fmt.Printf("Mapping Permission for %d repos\n", len(repoConfig))

	for _, v := range repoConfig {

		var loopList []PermStruct
		if permissionName == "robots" {
			loopList = v.PermissionList.Robots
		} else {
			loopList = v.PermissionList.Teams
		}
		fmt.Printf("Creating permission for repo: %s kind %s\n", v.Name, permissionName)
		for _, vv := range loopList {
			permList = append(permList, PermStruct{ Name: vv.Name, Role: vv.Role, PermissionKind: permissionName, RepoName: v.Name })	
		}
	}
	return
}

func createRobotTeam(quayHost string, orgName string, robotList []RobotStruct, teamList []TeamStruct, token string)(status bool){
	var wg sync.WaitGroup
	queueLength := 0
// robot
	fmt.Println("creating ", len(robotList), "robots")
	for _, v := range robotList {
		fmt.Println("Creating robot", v)
		queueLength++
		for queueLength > parallel {
			fmt.Println("sleep 1")
			time.Sleep(1 * time.Second)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			apiCall(quayHost, "/api/v1/organization/"+orgName+"/robots/"+ v.Name, "PUT", token, `{"description":"`+ v.Description +`"}`, "create robot "+ v.Name)
		}(&queueLength) 
	}
// teams
	fmt.Println("creating ", len(teamList), "teams")
	for _, v := range teamList {
		fmt.Println("Creating team", v)
		queueLength++
		for queueLength > parallel {
			fmt.Println("sleep 1")
			time.Sleep(1 * time.Second)
		}
		wg.Add(1)
		go func(counter *int) {
			defer wg.Done()
			*counter--
			apiCall(quayHost, "/api/v1/organization/"+orgName+"/team/"+ v.Name, "PUT", token, `{"role":"`+ v.Role +`"}`, "create team "+ v.Name)
			// apiCall(quayHost, "/api/v1/organization/"+orgName+"/team/"+ v.Name +"/syncing", "POST", token, `{"group_dn":"`+ v.GroupDN +`"}`, "create team sync "+ v.Name)
		}(&queueLength) 
	}
	wg.Wait()
	status = true
	return
}

func main() {
	var quays Quays
	var org Organization
	var permList []PermStruct
	var parsedOrg []Organization

	t1 := time.Now()
	
	var(
		quaysfile string
		repo []string
	)

	flag.Func("repo", "quay repo file name", func(s string) error {
		_, err := os.Stat(s)
		if err == nil {
			repo = append(repo, s)
			return nil
		} else {
			return errors.New("file does not exists")
		}
	})
	flag.StringVar(&quaysfile, "quaysfile", "" , "quay token file name")
	flag.IntVar(&parallel, "parallel", 50, "max parallel requests")
	flag.BoolVar(&insecure, "insecure", false, "disable TLS connection")

	flag.Parse()
	fmt.Println(len(repo))
	for _, v := range repo {
			fmt.Printf("- Name: %s\n", v)
	}
	fmt.Printf("quayfile %s\n\nparallel %d insecure %t\n", quaysfile, parallel, insecure)

	yamlData, err := os.ReadFile(quaysfile)
	
	if err != nil {
		log.Fatal("Error while reading token file", err)
	}

	yaml.Unmarshal(yamlData, &quays)

	for _, r := range repo {
		yamlData, err = os.ReadFile(r)	
		if err != nil {
			log.Fatal("Error while reading token file", err)
		}
		yaml.Unmarshal(yamlData, &org)
		parsedOrg = append(parsedOrg, org)
	}

	// for _, ooo := range parsedOrg {
	// 	for i, v := range ooo.RepoList {
	// 		fmt.Printf("Repository n.%d - Name: %s\n", i, v.Name)
	//  	}
	// }

	// for i, v := range token.HostToken {
	// 	fmt.Printf("token n.%d\n\t- Host: %s \n\t- token: %s\n", i, v.Host, v.Token)
	// }

	// for i, v := range org.RepoList {
	// 	fmt.Printf("Repository n.%d - Name: %s\n", i, v.Name)
	// }

	if ! checkLogin(quays){
		log.Fatal("Error logging to quay hosts")
	}

	// os.Exit(0)

	var wg sync.WaitGroup
	for _, v := range quays.HostToken {
		for _, o := range parsedOrg {
			wg.Add(1)
			go func() {
				defer wg.Done()
				fmt.Printf("creating organization - Host: %s \n\t- %s\n", v.Host, o.Name)
				createOrg(v.Host, o, v.Token)
				fmt.Printf("creating robots and teams for organization %s - Host: %s\n", o.Name, v.Host)
				createRobotTeam(v.Host, o.Name, o.RobotList, o.TeamsList, v.Token)
				fmt.Printf("creating repositories for organization %s - Host: %s\n", o.Name, v.Host)
				createRepo(v.Host, o.Name, o.RepoList, v.Token)
				permList = slices.Concat(permList, createPermissionList(o.RepoList, "robots"), createPermissionList(o.RepoList, "teams"))
				createRepoPermission(v.Host, o.Name, permList, v.Token)
				fmt.Printf("Repliquay: quay %s organization %s replicated in %s\n", v.Host, o.Name, time.Since(t1))
			}()
		}
	}
	wg.Wait()

	fmt.Printf("Repliquay: mission completed in %s\n", time.Since(t1))
}

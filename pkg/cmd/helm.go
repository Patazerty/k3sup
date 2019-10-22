package cmd

import (
	"fmt"
	"os"
	"path"
	"strconv"

	"github.com/spf13/cobra"

        "github.com/alexellis/k3sup/pkg/config"
)

func addTillerSA(accountName string, namespace string) error {
	if namespace != "kube-system" {
		if _, err := kubectlTask("create", "namespace", namespace); err != nil {
			return err
		}
	}
	if _, err := kubectlTask("-n", namespace, "create", "sa", accountName); err != nil {
		return err
	} else if _, err = kubectlTask("-n", namespace, "create", "clusterrolebinding", accountName, "--clusterrole",
		"cluster-admin", "--serviceaccount="+namespace+":"+accountName); err != nil {
		return err
	}
	return nil
}

func addTillerRestrictedSA(ns string, otherns string) error {
	var argsList [][]string
	if len(otherns) == 0 {
		argsList = [][]string{
			//Create ns
			{"create", "namespace", ns},
			//Create serviceaccount
			{"-n", ns, "create", "sa", "tiller"},
			//Create role
			{"-n", ns, "create", "role", "tiller-manager", "--verb=*",
				"--resource=*.,*.apps,*.batch,*.extensions"},
			//Create rolebinding
			{"-n", ns, "create", "rolebinding", "tiller-binding", "--role=tiller-manager",
				"--serviceaccount="+ns+":tiller"},
		}
	} else {
		argsList = [][]string{
			//Create nses
			{"create", "namespace", ns},
			{"create", "namespace", otherns},
			//Create serviceaccount
			{"-n", ns, "create", "sa", "tiller"},
			//Create role
			{"-n", otherns, "create", "role", "tiller-manager", "--verb=*",
				"--resource=*.,*.apps,*.batch,*.extensions"},
			//Bind it
			{"-n", otherns, "create", "rolebinding", "tiller-binding", "--role=tiller-manager",
				"--serviceaccount="+ns+":tiller"},
			//Create configmap access role
			{"-n", ns, "create", "role", "tiller-manager", "--verb=*",
				"--resource=configmaps"},
			//Bind it
			{"-n", ns, "create", "rolebinding", "tiller-binding", "--role=tiller-manager",
				"--serviceaccount="+ns+":tiller"},
		}
	}
	for _, args := range(argsList) {
		if _, err := kubectlTask(args...); err != nil {
			return err
		}
	}
	return nil
}

func MakeHelm() *cobra.Command {

	var command = &cobra.Command{
                Use:          "helm2",
                Short:        "Manage helm",
                Long:         `Manage helm`,
                Example:      `  helm init`,
                SilenceUsage: false,
        }

	var init = &cobra.Command{
                Use:          "init",
                Short:        "Install helm with tiller (helm v2 only)",
                Long:         `Install helm with tiller (helm v2 only)`,
                Example:      `  k3sup helm2 init insecure`,
                SilenceUsage: true,
        }

	var initInsecure = &cobra.Command{
                Use:          "insecure",
                Short:        "Install helm with insecure tiller",
                Long:         `Deploy tiller in kube-system namespace with cluster-admin role, no TLS and plaintext configmap storage`,
                Example:      `  k3sup helm2 init insecure`,
                SilenceUsage: true,

	}

	var initRestricted = &cobra.Command{
		Use:          "restricted",
                Short:        "Install helm with tiller restricted to a namespace",
                Long:         `Install helm with tiller restricted to a namespace`,
                Example:      `  k3sup helm2 init restricted --same-ns --namespace some-namespace`,
                SilenceUsage: true,
	}

	initRestricted.Flags().String("namespace", "ketchup", "Namespace of tiller")

	initRestricted.Flags().Bool("same-ns", true, "Set RBAC permissions to restrict tiller to be able to deploy in the same namespace it is deployed in")
	initRestricted.Flags().String("other-ns", "", "Set RBAC permissions to restrict tiller to deploy only in the specified namespace")

	initRestricted.Flags().Bool("secret-storage", true, "Use secret storage for tiller")
	initRestricted.Flags().Int("history-max", 200, "limit the maximum number of revisions saved per release. Use 0 for no limit")
	init.AddCommand(initInsecure)
	init.AddCommand(initRestricted)
	command.AddCommand(init)


	init.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			fmt.Println("You can use: k3sup helm init insecure, k3sup helm init restricted")
		}
		return nil
	}

	initInsecure.RunE = func(cmd *cobra.Command, args []string) error {
		exec_args := []string{}

		userPath, err := config.InitUserDir()
                if err != nil {
                        return err
                }
		os.Setenv("HELM_HOME", path.Join(userPath, ".helm"))


		exec_args = append(exec_args, "init", "--service-account", "tiller")
		if err := addTillerSA("tiller", "kube-system"); err != nil {
			return err
		}
		return helmInit(exec_args)
	}

	initRestricted.RunE = func(cmd *cobra.Command, args []string) error {
		exec_args := []string{"init"}

		userPath, err := config.InitUserDir()
                if err != nil {
                        return err
                }
		os.Setenv("HELM_HOME", path.Join(userPath, ".helm"))

		ns, err := cmd.Flags().GetString("namespace")
		if err != nil {
			return err
		}
		exec_args = append(exec_args, "--tiller-namespace", ns)

		if ok, err := cmd.Flags().GetBool("secret-storage"); err != nil && ok {
			exec_args = append(exec_args, "--override", "'spec.template.spec.containers[0].command'='{/tiller,--storage=secret}'")
		}

		if arg, err := cmd.Flags().GetInt("history-max"); err != nil {
			return err
		} else {
			exec_args = append(exec_args, "--history-max", strconv.Itoa(arg))
		}

		if cmd.Flags().Changed("other-ns") {
			if otherns, err := cmd.Flags().GetString("other-ns"); err != nil {
				return err
			} else {
				if err := addTillerRestrictedSA(ns, otherns); err != nil {
					return err
				}
			}
		} else {
			ok, err := cmd.Flags().GetBool("same-ns")
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("You need to specify --same-ns (default) or --other-ns <some-namespace>")
				return nil
			}
			if err := addTillerRestrictedSA(ns, ""); err != nil {
				return err
			}
		}
		exec_args = append(exec_args, "--service-account", "tiller", "--upgrade")
		return helmInit(exec_args)
	}
	return command
}

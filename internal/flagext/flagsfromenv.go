package flagext

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

func ParseFlagsAndEnvironment(flags *flag.FlagSet, envVarPrefix string) (err error) {
	supportedEnvVars := map[string]struct{}{}
	// Parse env var values into flags
	flags.VisitAll(func(f *flag.Flag) {
		envVarName := envVarPrefix + strings.ToUpper(strings.ReplaceAll(f.Name, "-", "_"))
		f.Usage = fmt.Sprintf("%s (%s)", f.Usage, envVarName)
		supportedEnvVars[envVarName] = struct{}{}
		if envVarValue := os.Getenv(envVarName); envVarValue != "" {
			f.DefValue = envVarValue
			e := f.Value.Set(envVarValue)
			if e != nil && err == nil {
				err = fmt.Errorf("invalid environment variable %s value provided: %w", envVarName, e)
			}
		}
	})
	if err != nil {
		return err
	}
	err = flags.Parse(os.Args[1:])
	if err != nil {
		return err
	}
	// Fail if unsupported environment variable provided
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, envVarPrefix) {
			kv := strings.SplitN(entry, "=", 2)
			if _, ok := supportedEnvVars[kv[0]]; !ok {
				return fmt.Errorf("unsupported environment variable provided: %s", kv[0])
			}
		}
	}
	return nil
}

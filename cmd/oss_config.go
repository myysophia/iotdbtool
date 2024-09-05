package cmd

import (
    "bufio"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/spf13/viper"
)

const configFileName = ".iotdbtools.config"

func configureOSS() {
    reader := bufio.NewReader(os.Stdin)

    fmt.Println("请配置 OSS 访问信息：")

    fmt.Print("请输入 AccessKey ID: ")
    ak, _ := reader.ReadString('\n')
    ak = strings.TrimSpace(ak)

    fmt.Print("请输入 AccessKey Secret: ")
    sk, _ := reader.ReadString('\n')
    sk = strings.TrimSpace(sk)

    fmt.Print("请输入 OSS Endpoint: ")
    endpoint, _ := reader.ReadString('\n')
    endpoint = strings.TrimSpace(endpoint)

    homeDir, err := os.UserHomeDir()
    if err != nil {
        fmt.Printf("无法获取用户主目录: %v\n", err)
        return
    }

    configPath := filepath.Join(homeDir, ".iotdbtools.config")

    viper.SetConfigFile(configPath)
    viper.SetConfigType("yaml")
    viper.Set("OSS_AK", ak)
    viper.Set("OSS_SK", sk)
    viper.Set("OSS_ENDPOINT", endpoint)

    err = viper.WriteConfig()
    if err != nil {
        fmt.Printf("无法保存配置: %v\n", err)
        return
    }

    fmt.Println("OSS 配置已保存")
}

func checkOSSConfig() {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        fmt.Printf("无法获取用户主目录: %v\n", err)
        return
    }

    configPath := filepath.Join(homeDir, configFileName)

    viper.SetConfigFile(configPath)
    err = viper.ReadInConfig()
    if err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); ok {
            fmt.Println("未找到 OSS 配置文件，请使用 --ossconf 参数进行配置")
        } else {
            fmt.Printf("读取配置文件错误: %v\n", err)
        }
        return
    }

    if !viper.IsSet("OSS_AK") || !viper.IsSet("OSS_SK") || !viper.IsSet("OSS_ENDPOINT") {
        fmt.Println("OSS 配置不完整，请使用 --ossconf 参数重新配置")
    }
}

func getOSSConfig() (string, string, string, error) {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return "", "", "", fmt.Errorf("无法获取用户主目录: %v", err)
    }

    configPath := filepath.Join(homeDir, configFileName)

    viper.SetConfigFile(configPath)
    viper.SetConfigType("yaml") // 明确指定配置文件类型为 YAML

    err = viper.ReadInConfig()
    if err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); ok {
            return "", "", "", fmt.Errorf("OSS 配置文件不存在，请使用 --ossconf 参数进行配置")
        }
        return "", "", "", fmt.Errorf("读取配置文件错误: %v", err)
    }

    ak := viper.GetString("OSS_AK")
    sk := viper.GetString("OSS_SK")
    endpoint := viper.GetString("OSS_ENDPOINT")

    if ak == "" || sk == "" || endpoint == "" {
        return "", "", "", fmt.Errorf("OSS 配置不完整，请使用 --ossconf 参数重新配置")
    }

    return ak, sk, endpoint, nil
}

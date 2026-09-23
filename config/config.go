package config

import (
    "log/slog"
    "gopkg.in/ini.v1"
)

type Config struct {
    Servidor struct {
        PuertoLocal      int `ini:"PuertoLocal"`
        MaxMessageSizeKB int `ini:"MaxMessageSizeKB"`
    } `ini:"Servidor"`

    Destino struct {
        Servidor string `ini:"Servidor"`
        Puerto   int    `ini:"Puerto"`
        Usuario  string `ini:"Usuario"`
        Password string `ini:"Password"`
        UsarAuth bool   `ini:"UsarAuth"`   // acepta 0/1/true/false
        UsarTLS  bool   `ini:"UsarTLS"`
        TipoTLS  string `ini:"TipoTLS"`    // "explicit" | "implicit"
    } `ini:"Destino"`

    Cola struct {
        IntervaloWorkerMs int `ini:"IntervaloWorkerMs"`
        MaxIntentos       int `ini:"MaxIntentos"`
        DiasMaxReintento  int `ini:"DiasMaxReintento"`
    } `ini:"Cola"`

    Archivos struct {
        RutaDB  string `ini:"RutaDB"`
        RutaLog string `ini:"RutaLog"`
    } `ini:"Archivos"`

    Log struct {
        Nivel int `ini:"Nivel"` // 0=Debug 1=Info 2=Warn 3=Error
    } `ini:"Log"`

    Web struct {
        Habilitado bool   `ini:"Habilitado"`
        Puerto     int    `ini:"Puerto"`
        Bind       string `ini:"Bind"`
        Usuario    string `ini:"Usuario"`
        Password   string `ini:"Password"`
    } `ini:"Web"`
}

func Load(path string) (*Config, error) {
    cfg := &Config{}
    f, err := ini.Load(path)
    if err != nil {
        return nil, err
    }
    if err := f.MapTo(cfg); err != nil {
        return nil, err
    }
    slog.Info("config cargada", "path", path)
    return cfg, nil
}
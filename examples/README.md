# Argus Examples

Questa cartella contiene esempi pratici di utilizzo di **Argus Panoptes**, il sistema di monitoraggio file ultra-performante di AGILira.

## 🎯 Esempio Principale: Iris Integration

### `example_iris_integration.go`

Dimostra l'implementazione del **punto 4 di Gemini**: *"Dynamic log level changes at runtime"*

**Features dimostrate:**
- ✅ Monitoraggio automatico file di configurazione JSON
- ✅ Aggiornamento dinamico del log level in tempo reale
- ✅ Audit trail completo per compliance e sicurezza
- ✅ Performance ultra-ottimizzate (4.166ns format detection)
- ✅ Support multi-formato universale

**Come eseguire:**
```bash
cd examples
go run example_iris_integration.go
```

**Output atteso:**
```
🎯 Demo: Argus + Iris Dynamic Log Level Changes
===================================================
📄 Created config file: /tmp/iris_config.json
🔍 Starting Argus watcher with audit trail...

🧪 Testing logs with initial level (info):
[INFO] This is an info message

🔄 Changing log level to 'debug' in config file...
📝 Iris log level changed: info -> debug
🌐 Port updated to: 9090
⚙️  Full config: map[enable_audit:true log_level:debug max_file_size:1.048576e+07 port:9090]

🧪 Testing logs with new level (debug):
[INFO] This is an info message
[DEBUG] This debug message SHOULD appear now!

📋 Audit Trail Summary:
=======================
{"timestamp":"2025-08-24T...","level":0,"event":"watch_start",...}
{"timestamp":"2025-08-24T...","level":2,"event":"config_change",...}
```

## 🚀 Performance Benchmarks

Il sistema Argus raggiunge prestazioni record:
- **4.166ns** - Format detection singolo
- **10.95ns** - Cache access lock-free  
- **1962ns** - Polling ottimizzato
- **1688ns** - Parsing configurazioni

## 🔒 Audit Trail

Ogni operazione viene tracciata con:
- Timestamp ultra-precisi (go-timecache)
- Checksums per tamper detection
- Before/after values per config changes
- Process ID e metadata di sicurezza

## 🌍 Formati Supportati

- JSON
- YAML (.yml, .yaml)
- TOML
- HCL (.hcl, .tf)
- INI (.ini, .conf, .cfg)
- Properties

## 📁 Altri Esempi

### `custom_parser/`
**Nuovo!** Dimostra come creare e registrare parser personalizzati per parsing di configurazione production-ready.

**Features dimostrate:**
- ✅ Implementazione parser personalizzati
- ✅ Registrazione parser (manuale e auto-registrazione)
- ✅ Confronto parser built-in vs personalizzati
- ✅ Ricaricamento configurazione live
- ✅ Architettura plugin per produzione

**Come eseguire:**
```bash
cd custom_parser
go run main.go
```

### `error_handling/`
Strategie complete di gestione errori con Argus.

**Features dimostrate:**
- ✅ Error handler personalizzati
- ✅ Gestione errori lettura file
- ✅ Gestione errori parsing
- ✅ Integrazione audit logging

### `iris_integration/`
Esempi specifici per integrazione con Iris

### `universal_demo/`
Demo formati universali

### `universal_formats/`
Test multi-formato

---

*Copyright (c) 2025 AGILira - Series: AGILira System Libraries*

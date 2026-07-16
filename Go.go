package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const dbFile = "comanda.db"

// Lock global para emular LockService de Google Apps Script
var scriptLock sync.Mutex

// --- ESTRUCTURAS DE DATOS ---

type Item struct {
	LineID      string  `json:"lineId"`
	Nombre      string  `json:"nombre"`
	Precio      float64 `json:"precio"`
	PrecioFinal float64 `json:"precioFinal"`
	Cantidad    int     `json:"cantidad"`
	Listo       bool    `json:"listo"`
	Specs       string  `json:"specs"`
	NotaItem    string  `json:"notaItem"`
}

type Evento struct {
	EventoID string `json:"eventoId"`
	Tipo     string `json:"tipo"`
	TS       int64  `json:"ts"`
	Autor    string `json:"autor"`
	Item     *Item  `json:"item,omitempty"`
	LineID   string `json:"lineId,omitempty"`
	Listo    bool   `json:"listo,omitempty"`
	Mesa     string `json:"mesa,omitempty"`
	Mesero   string `json:"mesero,omitempty"`
	Notas    string `json:"notas,omitempty"`
}

type Personal struct {
	Nombre      string   `json:"nombre"`
	Locales     []string `json:"locales"`
	Capacidades []string `json:"capacidades"`
	Activo      bool     `json:"activo"`
}

type PedidoOut struct {
	ID           string   `json:"id"`
	Fecha        string   `json:"fecha"`
	Hora         string   `json:"hora"`
	Mesa         string   `json:"mesa"`
	Mesero       string   `json:"mesero"`
	Items        string   `json:"items"`
	Total        float64  `json:"total"`
	MetodoPago   string   `json:"metodoPago"`
	Estado       string   `json:"estado"`
	Notas        string   `json:"notas"`
	HoraFin      *int64   `json:"horaFin"`
	NumeroPedido *int     `json:"numeroPedido"`
	Version      int      `json:"version"`
	ItemsRaw     string   `json:"itemsRaw"`
	CreadoEn     *int64   `json:"creadoEn"`
	Eventos      []Evento `json:"eventos"`
	Local        string   `json:"local"`
}

type Producto struct {
	ID        string  `json:"id"`
	Categoria string  `json:"categoria"`
	Nombre    string  `json:"nombre"`
	Precio    float64 `json:"precio"`
	ImgPostre string  `json:"imgPostre"`
	Activo    bool    `json:"activo"`
}

// Payload genérico de entrada para emular POST de GAS
type RequestData struct {
	Accion     string   `json:"accion"`
	Codigo     string   `json:"codigo"`
	Local      string   `json:"local"`
	Id         string   `json:"id"`
	Mesa       string   `json:"mesa"`
	Mesero     string   `json:"mesero"`
	Items      string   `json:"items"`
	ItemsRaw   string   `json:"itemsRaw"`
	MetodoPago string   `json:"metodoPago"`
	Notas      string   `json:"notas"`
	Estado     string   `json:"estado"`
	HoraFin    int64    `json:"horaFin"`
	Eventos    []Evento `json:"eventos"`
	Autor      string   `json:"autor"`
}

// Estructura de respuesta unificada compatible con tu Frontend actual
type Response struct {
	Ok           bool        `json:"ok"`
	Error        string      `json:"error,omitempty"`
	ID           string      `json:"id,omitempty"`
	NumeroPedido int         `json:"numeroPedido,omitempty"`
	Eventos      []Evento    `json:"eventos,omitempty"`
	Items        string      `json:"items,omitempty"`
	ItemsRaw     []Item      `json:"itemsRaw,omitempty"`
	Total        float64     `json:"total,omitempty"`
	Mesa         string      `json:"mesa,omitempty"`
	Mesero       string      `json:"mesero,omitempty"`
	Notas        string      `json:"notas,omitempty"`
	Pedidos      []PedidoOut `json:"pedidos,omitempty"`
	Productos    []Producto  `json:"productos,omitempty"`
	Nombre       string      `json:"nombre,omitempty"`
	Local        string      `json:"local,omitempty"`
	Locales      []string    `json:"locales,omitempty"`
	Capacidades  []string    `json:"capacidades,omitempty"`
	EsAdmin      bool        `json:"esAdmin,omitempty"`
}

// --- UTILIDADES ---

func newUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func splitAndTrim(s string) []string {
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	var res []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

func coalesce(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// --- BASE DE DATOS E INICIALIZACIÓN ---

func initDB() (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbFile)
	if err != nil {
		return nil, err
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS pedidos (
			id TEXT PRIMARY KEY, fecha TEXT, hora TEXT, mesa TEXT, mesero TEXT, items TEXT,
			total REAL, metodo_pago TEXT, estado TEXT, notas TEXT, archivado TEXT,
			hora_fin INTEGER, numero_pedido INTEGER, version INTEGER, items_raw TEXT,
			historial TEXT, creado_en INTEGER, eventos TEXT, local TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS menu (
			id TEXT PRIMARY KEY, categoria TEXT, nombre TEXT, precio REAL, img_postre TEXT, activo TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS personal (
			codigo TEXT PRIMARY KEY, nombre TEXT, locales TEXT, capacidades TEXT, activo TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS configuracion (
			local TEXT PRIMARY KEY, apertura TEXT, cierre TEXT, bloqueo_activo TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS propiedades (
			clave TEXT PRIMARY KEY, valor TEXT
		);`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return nil, err
		}
	}

	seedData(db)
	return db, nil
}

func seedData(db *sql.DB) {
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM personal").Scan(&count)
	if count == 0 {
		_, _ = db.Exec("INSERT INTO personal VALUES (?, ?, ?, ?, ?)", "1234", "Mesero Estrella", "1", "crear,editar,ver,archivar", "SI")
		_, _ = db.Exec("INSERT INTO personal VALUES (?, ?, ?, ?, ?)", "0000", "Admin Boss", "1", "admin", "SI")
	}

	_ = db.QueryRow("SELECT COUNT(*) FROM menu").Scan(&count)
	if count == 0 {
		_, _ = db.Exec("INSERT INTO menu VALUES (?, ?, ?, ?, ?, ?)", "1", "Bebidas", "Agua de Horchata", 4500.0, "", "SI")
		_, _ = db.Exec("INSERT INTO menu VALUES (?, ?, ?, ?, ?, ?)", "2", "Comidas", "Tacos al Pastor", 12000.0, "", "SI")
	}

	_ = db.QueryRow("SELECT COUNT(*) FROM configuracion").Scan(&count)
	if count == 0 {
		_, _ = db.Exec("INSERT INTO configuracion VALUES (?, ?, ?, ?)", "1", "08:00", "23:59", "NO")
	}
}

func siguienteNumeroPedido(db *sql.DB) (int, error) {
	var valor string
	err := db.QueryRow("SELECT valor FROM propiedades WHERE clave = 'ULTIMO_NUMERO_PEDIDO'").Scan(&valor)
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO propiedades VALUES ('ULTIMO_NUMERO_PEDIDO', '1')")
		if err != nil {
			return 0, err
		}
		return 1, nil
	} else if err != nil {
		return 0, err
	}

	n, _ := strconv.Atoi(valor)
	n++
	_, err = db.Exec("UPDATE propiedades SET valor = ? WHERE clave = 'ULTIMO_NUMERO_PEDIDO'", strconv.Itoa(n))
	if err != nil {
		return 0, err
	}
	return n, nil
}

// --- LÓGICA DE CONTROL ---

func dentroDeHorario(db *sql.DB, local string) bool {
	var apertura, cierre, bloqueoActivo string
	err := db.QueryRow("SELECT apertura, cierre, bloqueo_activo FROM configuracion WHERE local = ?", local).Scan(&apertura, &cierre, &bloqueoActivo)
	if err != nil {
		return true
	}
	if strings.ToUpper(bloqueoActivo) != "SI" || apertura == "" || cierre == "" {
		return true
	}

	loc, err := time.LoadLocation("America/Bogota")
	if err != nil {
		loc = time.Local
	}
	ahora := time.Now().In(loc).Format("15:04")

	if apertura <= cierre {
		return ahora >= apertura && ahora <= cierre
	}
	return ahora >= apertura || ahora <= cierre
}

func verificarPermiso(db *sql.DB, codigo, capacidadRequerida, local string) (bool, string, *Personal) {
	var nombre, localesStr, capacidadesStr, activoStr string
	err := db.QueryRow("SELECT nombre, locales, capacidades, activo FROM personal WHERE codigo = ?", strings.TrimSpace(codigo)).Scan(&nombre, &localesStr, &capacidadesStr, &activoStr)
	if err == sql.ErrNoRows {
		return false, "Código no reconocido.", nil
	} else if err != nil {
		return false, "Error de base de datos.", nil
	}

	if strings.ToUpper(activoStr) == "NO" {
		return false, "Este código está desactivado.", nil
	}

	locales := splitAndTrim(localesStr)
	capacidades := splitAndTrim(capacidadesStr)

	persona := &Personal{
		Nombre:      nombre,
		Locales:     locales,
		Capacidades: capacidades,
		Activo:      true,
	}

	hasLocal := false
	for _, l := range locales {
		if l == local {
			hasLocal = true
			break
		}
	}
	if local != "" && !hasLocal {
		return false, "No tienes acceso a este local.", nil
	}

	isAdmin := false
	hasCap := false
	for _, c := range capacidades {
		if c == "admin" {
			isAdmin = true
		}
		if c == capacidadRequerida {
			hasCap = true
		}
	}

	if !hasCap && !isAdmin {
		return false, "No tienes permiso para esta acción.", nil
	}

	if !isAdmin && local != "" && !dentroDeHorario(db, local) {
		return false, "Fuera del horario permitido para este local.", nil
	}

	return true, "", persona
}

func validarAcceso(db *sql.DB, data RequestData) Response {
	ok, errStr, pers := verificarPermiso(db, data.Codigo, "ver", data.Local)
	if !ok {
		return Response{Ok: false, Error: errStr}
	}

	return Response{
		Ok:          true,
		Nombre:      pers.Nombre,
		Local:       data.Local,
		Locales:     pers.Locales,
		Capacidades: pers.Capacidades,
		EsAdmin:     false, // Puede definirse evaluando si tiene la capacidad "admin"
	}
}

// --- ACCIONES CORE DEL SISTEMA (IDEMPOTENTES) ---

func guardarPedido(db *sql.DB, data RequestData) Response {
	scriptLock.Lock()
	defer scriptLock.Unlock()

	ok, errStr, _ := verificarPermiso(db, data.Codigo, "crear", data.Local)
	if !ok {
		return Response{Ok: false, Error: errStr}
	}

	id := data.Id
	if id == "" {
		id = newUUID()
	}

	// Idempotencia
	var existingNum int
	err := db.QueryRow("SELECT numero_pedido FROM pedidos WHERE id = ?", id).Scan(&existingNum)
	if err == nil {
		return Response{Ok: true, ID: id, NumeroPedido: existingNum}
	}

	loc, _ := time.LoadLocation("America/Bogota")
	if loc == nil {
		loc = time.Local
	}
	ahora := time.Now().In(loc)
	fechaTxt := ahora.Format("02/01/2006")
	horaTxt := ahora.Format("15:04:05")
	ts := ahora.UnixNano() / int64(time.Millisecond)

	numeroPedido, err := siguienteNumeroPedido(db)
	if err != nil {
		return Response{Ok: false, Error: err.Error()}
	}

	var itemsIniciales []Item
	if data.ItemsRaw != "" {
		_ = json.Unmarshal([]byte(data.ItemsRaw), &itemsIniciales)
	}

	var total float64
	for _, it := range itemsIniciales {
		precio := it.PrecioFinal
		if precio == 0 {
			precio = it.Precio
		}
		cant := it.Cantidad
		if cant == 0 {
			cant = 1
		}
		total += precio * float64(cant)
	}

	var eventos []Evento
	for _, it := range itemsIniciales {
		eventos = append(eventos, Evento{
			EventoID: newUUID(),
			Tipo:     "item_agregado",
			TS:       ts,
			Autor:    coalesce(data.Mesero, "—"),
			Item:     &it,
		})
	}
	eventos = append(eventos, Evento{
		EventoID: newUUID(),
		Tipo:     "meta_actualizada",
		TS:       ts,
		Autor:    coalesce(data.Mesero, "—"),
		Mesa:     coalesce(data.Mesa, "—"),
		Mesero:   coalesce(data.Mesero, "—"),
		Notas:    data.Notas,
	})

	eventosJSON, _ := json.Marshal(eventos)
	itemsRawJSON, _ := json.Marshal(itemsIniciales)
	historial := fmt.Sprintf("[%s] Creado por %s", horaTxt, coalesce(data.Mesero, "—"))

	_, err = db.Exec(`INSERT INTO pedidos VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, fechaTxt, horaTxt, coalesce(data.Mesa, "—"), coalesce(data.Mesero, "—"), data.Items, total,
		coalesce(data.MetodoPago, "Por cobrar"), "Pendiente", data.Notas, "NO", 0, numeroPedido, 1,
		string(itemsRawJSON), historial, ts, string(eventosJSON), coalesce(data.Local, "1"),
	)
	if err != nil {
		return Response{Ok: false, Error: "Error al guardar: " + err.Error()}
	}

	return Response{Ok: true, ID: id, NumeroPedido: numeroPedido, Eventos: eventos}
}

func registrarEventos(db *sql.DB, data RequestData) Response {
	scriptLock.Lock()
	defer scriptLock.Unlock()

	ok, errStr, _ := verificarPermiso(db, data.Codigo, "editar", data.Local)
	if !ok {
		return Response{Ok: false, Error: errStr}
	}

	var originalLocal, eventosStr, originalMesa, originalMesero, originalNotas, originalHistorial string
	var originalVersion int
	err := db.QueryRow("SELECT local, eventos, mesa, mesero, notas, version, historial FROM pedidos WHERE id = ?", data.Id).Scan(
		&originalLocal, &eventosStr, &originalMesa, &originalMesero, &originalNotas, &originalVersion, &originalHistorial,
	)
	if err == sql.ErrNoRows {
		return Response{Ok: false, Error: "Pedido no encontrado."}
	} else if err != nil {
		return Response{Ok: false, Error: err.Error()}
	}

	reqLocal := coalesce(data.Local, "1")
	dbLocal := coalesce(originalLocal, "1")
	if reqLocal != dbLocal {
		return Response{Ok: false, Error: "Este pedido no pertenece a tu local."}
	}

	var eventosGuardados []Evento
	_ = json.Unmarshal([]byte(eventosStr), &eventosGuardados)

	idsExistentes := make(map[string]bool)
	for _, ev := range eventosGuardados {
		idsExistentes[ev.EventoID] = true
	}

	var nuevosEventos []Evento
	for _, ev := range data.Eventos {
		if ev.EventoID != "" && !idsExistentes[ev.EventoID] {
			nuevosEventos = append(nuevosEventos, ev)
		}
	}

	todosLosEventos := append(eventosGuardados, nuevosEventos...)
	sort.Slice(todosLosEventos, func(i, j int) bool {
		return todosLosEventos[i].TS < todosLosEventos[j].TS
	})

	// RECALCULAR ESTADO ACTUAL MEDIANTE EL FOLD (TU GRAN DISEÑO)
	type LineState struct {
		Item      Item
		Eliminado bool
	}
	itemsPorLinea := make(map[string]*LineState)
	metaMesa := originalMesa
	metaMesero := originalMesero
	metaNotas := originalNotas
	var metaTs int64 = -1

	for _, ev := range todosLosEventos {
		switch ev.Tipo {
		case "item_agregado":
			if ev.Item != nil && ev.Item.LineID != "" {
				itemsPorLinea[ev.Item.LineID] = &LineState{Item: *ev.Item, Eliminado: false}
			}
		case "item_modificado":
			if ev.Item != nil && ev.Item.LineID != "" {
				if state, exists := itemsPorLinea[ev.Item.LineID]; exists && !state.Eliminado {
					state.Item = *ev.Item
				} else if !exists {
					itemsPorLinea[ev.Item.LineID] = &LineState{Item: *ev.Item, Eliminado: false}
				}
			}
		case "item_eliminado":
			if ev.LineID != "" {
				if state, exists := itemsPorLinea[ev.LineID]; exists {
					state.Eliminado = true
				}
			}
		case "item_listo":
			if ev.LineID != "" {
				if state, exists := itemsPorLinea[ev.LineID]; exists {
					state.Item.Listo = ev.Listo
				}
			}
		case "meta_actualizada":
			if ev.TS >= metaTs {
				metaMesa = ev.Mesa
				metaMesero = ev.Mesero
				metaNotas = ev.Notas
				metaTs = ev.TS
			}
		}
	}

	var itemsActivos []Item
	for _, state := range itemsPorLinea {
		if !state.Eliminado {
			itemsActivos = append(itemsActivos, state.Item)
		}
	}

	var total float64
	var itemsTxtParts []string
	for _, it := range itemsActivos {
		precio := it.PrecioFinal
		if precio == 0 {
			precio = it.Precio
		}
		cant := it.Cantidad
		if cant == 0 {
			cant = 1
		}
		total += precio * float64(cant)

		lineStr := fmt.Sprintf("%dx %s", cant, it.Nombre)
		if it.Specs != "" {
			lineStr += fmt.Sprintf(" [%s]", it.Specs)
		}
		if it.NotaItem != "" {
			lineStr += fmt.Sprintf(" (%s)", it.NotaItem)
		}
		itemsTxtParts = append(itemsTxtParts, lineStr)
	}
	textoItems := strings.Join(itemsTxtParts, ", ")

	todosEventosJSON, _ := json.Marshal(todosLosEventos)
	itemsActivosJSON, _ := json.Marshal(itemsActivos)
	nuevaVersion := originalVersion + 1

	nuevoHistorial := originalHistorial
	if len(nuevosEventos) > 0 {
		loc, _ := time.LoadLocation("America/Bogota")
		if loc == nil {
			loc = time.Local
		}
		horaTxt := time.Now().In(loc).Format("15:04:05")

		var resumenParts []string
		for _, e := range nuevosEventos {
			switch e.Tipo {
			case "item_agregado":
				nombre := "?"
				if e.Item != nil {
					nombre = e.Item.Nombre
				}
				resumenParts = append(resumenParts, "+ "+nombre)
			case "item_eliminado":
				resumenParts = append(resumenParts, "- ítem")
			case "item_listo":
				resumenParts = append(resumenParts, "check ítem")
			case "meta_actualizada":
				resumenParts = append(resumenParts, "edición de datos")
			default:
				resumenParts = append(resumenParts, e.Tipo)
			}
		}
		resumen := strings.Join(resumenParts, ", ")
		nuevoHistorial = fmt.Sprintf("%s\n[%s] %s: %s", originalHistorial, horaTxt, coalesce(data.Autor, "—"), resumen)
	}

	_, err = db.Exec(`UPDATE pedidos SET 
		mesa = ?, mesero = ?, notas = ?, items = ?, total = ?, items_raw = ?, eventos = ?, version = ?, historial = ? 
		WHERE id = ?`,
		metaMesa, metaMesero, metaNotas, textoItems, total, string(itemsActivosJSON), string(todosEventosJSON), nuevaVersion, nuevoHistorial, data.Id,
	)
	if err != nil {
		return Response{Ok: false, Error: "Error al actualizar eventos: " + err.Error()}
	}

	return Response{
		Ok:       true,
		Items:    textoItems,
		ItemsRaw: itemsActivos,
		Total:    total,
		Mesa:     metaMesa,
		Mesero:   metaMesero,
		Notas:    metaNotas,
		Eventos:  todosLosEventos,
	}
}

func cambiarEstado(db *sql.DB, data RequestData) Response {
	var originalHistorial string
	err := db.QueryRow("SELECT historial FROM pedidos WHERE id = ?", data.Id).Scan(&originalHistorial)
	if err == sql.ErrNoRows {
		return Response{Ok: false, Error: "Pedido no encontrado."}
	} else if err != nil {
		return Response{Ok: false, Error: err.Error()}
	}

	loc, _ := time.LoadLocation("America/Bogota")
	if loc == nil {
		loc = time.Local
	}
	horaTxt := time.Now().In(loc).Format("15:04:05")

	entrada := fmt.Sprintf("[%s] Estado -> %s", horaTxt, data.Estado)
	if data.MetodoPago != "" {
		entrada += fmt.Sprintf(" | Pago: %s", data.MetodoPago)
	}
	nuevoHistorial := fmt.Sprintf("%s\n%s", originalHistorial, entrada)

	if data.MetodoPago != "" && data.HoraFin != 0 {
		_, err = db.Exec("UPDATE pedidos SET estado = ?, metodo_pago = ?, hora_fin = ?, historial = ? WHERE id = ?", data.Estado, data.MetodoPago, data.HoraFin, nuevoHistorial, data.Id)
	} else if data.MetodoPago != "" {
		_, err = db.Exec("UPDATE pedidos SET estado = ?, metodo_pago = ?, historial = ? WHERE id = ?", data.Estado, data.MetodoPago, nuevoHistorial, data.Id)
	} else if data.HoraFin != 0 {
		_, err = db.Exec("UPDATE pedidos SET estado = ?, hora_fin = ?, historial = ? WHERE id = ?", data.Estado, data.HoraFin, nuevoHistorial, data.Id)
	} else {
		_, err = db.Exec("UPDATE pedidos SET estado = ?, historial = ? WHERE id = ?", data.Estado, nuevoHistorial, data.Id)
	}

	if err != nil {
		return Response{Ok: false, Error: err.Error()}
	}
	return Response{Ok: true}
}

func archivarPedido(db *sql.DB, data RequestData) Response {
	ok, errStr, _ := verificarPermiso(db, data.Codigo, "archivar", data.Local)
	if !ok {
		return Response{Ok: false, Error: errStr}
	}

	var originalLocal string
	err := db.QueryRow("SELECT local FROM pedidos WHERE id = ?", data.Id).Scan(&originalLocal)
	if err == sql.ErrNoRows {
		return Response{Ok: false, Error: "Pedido no encontrado."}
	}

	if coalesce(data.Local, "1") != coalesce(originalLocal, "1") {
		return Response{Ok: false, Error: "Este pedido no pertenece a tu local."}
	}

	_, err = db.Exec("UPDATE pedidos SET archivado = 'SI' WHERE id = ?", data.Id)
	if err != nil {
		return Response{Ok: false, Error: err.Error()}
	}

	return Response{Ok: true}
}

func obtenerPedidos(db *sql.DB, codigo, local string) Response {
	ok, errStr, _ := verificarPermiso(db, codigo, "ver", local)
	if !ok {
		return Response{Ok: false, Error: errStr}
	}

	rows, err := db.Query("SELECT id, fecha, hora, mesa, mesero, items, total, metodo_pago, estado, notas, hora_fin, numero_pedido, version, items_raw, creado_en, eventos, local FROM pedidos WHERE archivado = 'NO' AND (local = ? OR (local IS NULL AND ? = '1'))", local, local)
	if err != nil {
		return Response{Ok: false, Error: err.Error()}
	}
	defer rows.Close()

	var pedidos []PedidoOut
	for rows.Next() {
		var p PedidoOut
		var hFinVal, creadoEnVal int64
		var numPedVal int
		var eventosJSON string

		err = rows.Scan(&p.ID, &p.Fecha, &p.Hora, &p.Mesa, &p.Mesero, &p.Items, &p.Total, &p.MetodoPago, &p.Estado, &p.Notas, &hFinVal, &numPedVal, &p.Version, &p.ItemsRaw, &creadoEnVal, &eventosJSON, &p.Local)
		if err != nil {
			return Response{Ok: false, Error: err.Error()}
		}

		if hFinVal != 0 {
			p.HoraFin = &hFinVal
		}
		if numPedVal != 0 {
			p.NumeroPedido = &numPedVal
		}
		if creadoEnVal != 0 {
			p.CreadoEn = &creadoEnVal
		}
		_ = json.Unmarshal([]byte(eventosJSON), &p.Eventos)
		pedidos = append(pedidos, p)
	}

	if pedidos == nil {
		pedidos = []PedidoOut{}
	}
	return Response{Ok: true, Pedidos: pedidos}
}

func obtenerMenu(db *sql.DB) Response {
	rows, err := db.Query("SELECT id, categoria, nombre, precio, img_postre, activo FROM menu")
	if err != nil {
		return Response{Ok: false, Error: err.Error()}
	}
	defer rows.Close()

	var productos []Producto
	for rows.Next() {
		var p Producto
		var activoStr string
		err = rows.Scan(&p.ID, &p.Categoria, &p.Nombre, &p.Precio, &p.ImgPostre, &activoStr)
		if err != nil {
			return Response{Ok: false, Error: err.Error()}
		}
		p.Activo = strings.ToUpper(activoStr) != "NO"
		if p.Activo {
			productos = append(productos, p)
		}
	}

	if productos == nil {
		productos = []Producto{}
	}
	return Response{Ok: true, Productos: productos}
}

// --- SERVIDOR HTTP (ENTRY POINT) ---

func main() {
	db, err := initDB()
	if err != nil {
		log.Fatalf("Fallo de Base de Datos: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Habilitar CORS para conectar tus interfaces directo
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if r.Method == "POST" {
			var data RequestData
			if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
				json.NewEncoder(w).Encode(Response{Ok: false, Error: "JSON malformado: " + err.Error()})
				return
			}

			var resp Response
			switch data.Accion {
			case "validarAcceso":
				resp = validarAcceso(db, data)
			case "nuevoPedido":
				resp = guardarPedido(db, data)
			case "registrarEventos":
				resp = registrarEventos(db, data)
			case "cambiarEstado":
				resp = cambiarEstado(db, data)
			case "obtenerPedidos":
				resp = obtenerPedidos(db, data.Codigo, data.Local)
			case "archivarPedido":
				resp = archivarPedido(db, data)
			default:
				resp = Response{Ok: false, Error: "Acción no soportada: " + data.Accion}
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		if r.Method == "GET" {
			accion := r.URL.Query().Get("accion")
			codigo := r.URL.Query().Get("codigo")
			local := r.URL.Query().Get("local")

			var resp Response
			if accion == "obtenerPedidos" {
				resp = obtenerPedidos(db, codigo, local)
			} else if accion == "obtenerMenu" {
				resp = obtenerMenu(db)
			} else {
				resp = Response{Ok: false, Error: "Acción GET no soportada: " + accion}
			}
			json.NewEncoder(w).Encode(resp)
			return
		}

		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	fmt.Println("Servidor local Comanda Pro escuchando en http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

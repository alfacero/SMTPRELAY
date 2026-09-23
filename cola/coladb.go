package cola

import (
	"database/sql"
	_ "modernc.org/sqlite"
	"sync"
	"time"
)

type Estado int

const (
	Pendiente Estado = 0
	Enviando  Estado = 1
	Enviado   Estado = 2
	Fallido   Estado = 3
)

type Correo struct {
	ID             int64
	Remitente      string
	Destinatarios  string
	Asunto         string
	RawMessage     []byte
	Estado         Estado
	Intentos       int
	UltimoError    string
	ProximoIntento time.Time
	Creado         time.Time
	Enviado        sql.NullTime
}

type ColaDB struct {
	db *sql.DB
	mu sync.Mutex
}

func Abrir(ruta string) (*ColaDB, error) {
	db, err := sql.Open("sqlite", ruta+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	if err := crearEsquema(db); err != nil {
		return nil, err
	}
	return &ColaDB{db: db}, nil
}

func crearEsquema(db *sql.DB) error {
	_, err := db.Exec(`
        CREATE TABLE IF NOT EXISTS correos (
            id              INTEGER PRIMARY KEY AUTOINCREMENT,
            remitente       TEXT NOT NULL DEFAULT '',
            destinatarios   TEXT NOT NULL DEFAULT '',
            asunto          TEXT DEFAULT '',
            raw_message     BLOB NOT NULL,
            estado          INTEGER NOT NULL DEFAULT 0,
            intentos        INTEGER NOT NULL DEFAULT 0,
            ultimo_error    TEXT DEFAULT '',
            proximo_intento DATETIME,
            creado          DATETIME NOT NULL,
            enviado         DATETIME
        );
        CREATE INDEX IF NOT EXISTS idx_estado_prox ON correos(estado, proximo_intento);
    `)
	return err
}

// ---------------------------------------------------------------------------
// CONSULTAS PARA EL PANEL WEB
// ---------------------------------------------------------------------------

// ContarPorEstado devuelve cuántos correos hay en cada estado.
func (c *ColaDB) ContarPorEstado() (map[Estado]int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	rows, err := c.db.Query(`SELECT estado, COUNT(*) FROM correos GROUP BY estado`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[Estado]int{
		Pendiente: 0,
		Enviando:  0,
		Enviado:   0,
		Fallido:   0,
	}
	for rows.Next() {
		var est Estado
		var n int
		if err := rows.Scan(&est, &n); err != nil {
			return nil, err
		}
		out[est] = n
	}
	return out, rows.Err()
}

// Listar devuelve correos filtrados por estado (nil = todos), con paginación.
func (c *ColaDB) Listar(estado *Estado, offset, limit int) ([]Correo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	query := `SELECT id, remitente, destinatarios, asunto, estado, intentos,
                     ultimo_error, proximo_intento, creado, enviado
              FROM correos`
	args := []any{}

	if estado != nil {
		query += ` WHERE estado = ?`
		args = append(args, *estado)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Correo
	for rows.Next() {
		var co Correo
		var prox sql.NullTime
		var creado sql.NullTime
		if err := rows.Scan(
			&co.ID, &co.Remitente, &co.Destinatarios, &co.Asunto,
			&co.Estado, &co.Intentos, &co.UltimoError,
			&prox, &creado, &co.Enviado,
		); err != nil {
			return nil, err
		}
		if prox.Valid {
			co.ProximoIntento = prox.Time
		}
		if creado.Valid {
			co.Creado = creado.Time
		}
		out = append(out, co)
	}
	return out, rows.Err()
}

// ObtenerPorID devuelve un correo completo, incluyendo el raw_message.
func (c *ColaDB) ObtenerPorID(id int64) (*Correo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	row := c.db.QueryRow(`
        SELECT id, remitente, destinatarios, asunto, raw_message, estado,
               intentos, ultimo_error, proximo_intento, creado, enviado
        FROM correos WHERE id = ?`, id)

	var co Correo
	var prox, creado sql.NullTime
	if err := row.Scan(
		&co.ID, &co.Remitente, &co.Destinatarios, &co.Asunto, &co.RawMessage,
		&co.Estado, &co.Intentos, &co.UltimoError,
		&prox, &creado, &co.Enviado,
	); err != nil {
		return nil, err
	}
	if prox.Valid {
		co.ProximoIntento = prox.Time
	}
	if creado.Valid {
		co.Creado = creado.Time
	}
	return &co, nil
}

// ForzarReintento resetea un correo para que el worker lo tome ya mismo.
// Pone estado=pendiente, proximo_intento=ahora, y NO toca los intentos
// (para que el backoff no se reinicie).
func (c *ColaDB) ForzarReintento(id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`
        UPDATE correos SET estado = 0, proximo_intento = ?
        WHERE id = ?`, time.Now(), id)
	return err
}

// Eliminar borra un correo de la cola definitivamente.
func (c *ColaDB) Eliminar(id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`DELETE FROM correos WHERE id = ?`, id)
	return err
}

// ReintentarFallidos pone todos los fallidos definitivos como pendientes
// de nuevo (reseteando intentos). Útil después de arreglar un problema.
func (c *ColaDB) ReintentarFallidos() (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	res, err := c.db.Exec(`
        UPDATE correos
        SET estado = 0, intentos = 0, proximo_intento = ?, ultimo_error = ''
        WHERE estado = 3`, time.Now())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Buscar filtra por remitente, destinatario o asunto (LIKE).
func (c *ColaDB) Buscar(q string, offset, limit int) ([]Correo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	like := "%" + q + "%"
	rows, err := c.db.Query(`
        SELECT id, remitente, destinatarios, asunto, estado, intentos,
               ultimo_error, proximo_intento, creado, enviado
        FROM correos
        WHERE remitente LIKE ? OR destinatarios LIKE ? OR asunto LIKE ?
        ORDER BY id DESC LIMIT ? OFFSET ?`,
		like, like, like, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Correo
	for rows.Next() {
		var co Correo
		var prox, creado sql.NullTime
		if err := rows.Scan(
			&co.ID, &co.Remitente, &co.Destinatarios, &co.Asunto,
			&co.Estado, &co.Intentos, &co.UltimoError,
			&prox, &creado, &co.Enviado,
		); err != nil {
			return nil, err
		}
		if prox.Valid {
			co.ProximoIntento = prox.Time
		}
		if creado.Valid {
			co.Creado = creado.Time
		}
		out = append(out, co)
	}
	return out, rows.Err()
}

// Estadisticas arma un resumen para el dashboard.
type Estadisticas struct {
	PorEstado      map[Estado]int
	Total          int
	EnviadosHoy    int
	EnviadosSemana int
	TasaExito      float64 // 0..1
}

func (c *ColaDB) Estadisticas() (*Estadisticas, error) {
	porEstado, err := c.ContarPorEstado()
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	stats := &Estadisticas{PorEstado: porEstado}
	for _, n := range porEstado {
		stats.Total += n
	}

	// Enviados hoy
	hoy := time.Now().Truncate(24 * time.Hour)
	_ = c.db.QueryRow(`
        SELECT COUNT(*) FROM correos
        WHERE estado = 2 AND enviado >= ?`, hoy).Scan(&stats.EnviadosHoy)

	// Enviados últimos 7 días
	semana := time.Now().Add(-7 * 24 * time.Hour)
	_ = c.db.QueryRow(`
        SELECT COUNT(*) FROM correos
        WHERE estado = 2 AND enviado >= ?`, semana).Scan(&stats.EnviadosSemana)

	// Tasa de éxito = enviados / (enviados + fallidos)
	enviados := porEstado[Enviado]
	fallidos := porEstado[Fallido]
	if total := enviados + fallidos; total > 0 {
		stats.TasaExito = float64(enviados) / float64(total)
	}
	return stats, nil
}

func (c *ColaDB) Encolar(remitente, destinatarios, asunto string, raw []byte) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	res, err := c.db.Exec(`
        INSERT INTO correos (remitente, destinatarios, asunto, raw_message, estado, proximo_intento, creado)
        VALUES (?, ?, ?, ?, 0, ?, ?)`,
		remitente, destinatarios, asunto, raw, time.Now(), time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// TomarSiguientePendiente marca como "enviando" de forma atómica y devuelve el correo.
func (c *ColaDB) TomarSiguientePendiente() (*Correo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	tx, err := c.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
        SELECT id, remitente, destinatarios, asunto, raw_message, intentos
        FROM correos
        WHERE estado = 0 AND (proximo_intento IS NULL OR proximo_intento <= ?)
        ORDER BY id ASC LIMIT 1`, time.Now())

	var co Correo
	if err := row.Scan(&co.ID, &co.Remitente, &co.Destinatarios, &co.Asunto,
		&co.RawMessage, &co.Intentos); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if _, err := tx.Exec(`UPDATE correos SET estado = 1 WHERE id = ?`, co.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	co.Estado = Enviando
	return &co, nil
}

func (c *ColaDB) MarcarEnviado(id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`UPDATE correos SET estado = 2, enviado = ? WHERE id = ?`,
		time.Now(), id)
	return err
}

func (c *ColaDB) RegistrarFallo(id int64, intentos int, errMsg string, prox time.Time) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`
        UPDATE correos SET estado = 0, intentos = ?, ultimo_error = ?, proximo_intento = ?
        WHERE id = ?`, intentos, errMsg, prox, id)
	return err
}

func (c *ColaDB) MarcarFallidoDefinitivo(id int64, errMsg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`UPDATE correos SET estado = 3, ultimo_error = ? WHERE id = ?`,
		errMsg, id)
	return err
}

// RecuperarEnviando: al arrancar, cualquier correo en estado 1 (quedó a medias) vuelve a 0.
func (c *ColaDB) RecuperarEnviando() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.db.Exec(`UPDATE correos SET estado = 0 WHERE estado = 1`)
	return err
}

func (c *ColaDB) Cerrar() error { return c.db.Close() }

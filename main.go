package main

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	screenW, screenH = 960, 640
	worldW, worldH   = 2600, 1900
	shardCount       = 28
	magneticRange    = 220.0
)

type vector struct{ x, y float64 }
type shard struct {
	position, velocity vector
	polarity           int
	active, held       bool
}
type enemy struct {
	position          vector
	health, maxHealth int
	kind              int
	hitFlash          float64
	active            bool
}
type projectile struct {
	position, velocity vector
	active             bool
}
type resourceNode struct {
	position vector
	amount   int
	active   bool
}
type turret struct {
	position vector
	cooldown int
	active   bool
}
type game struct {
	player                                                    vector
	shards                                                    []shard
	enemies                                                   []enemy
	resources                                                 []resourceNode
	turrets                                                   []turret
	projectiles                                               []projectile
	polarity, score, combo, bestCombo, wave, heldShard, scrap int
	charge, pulse, comboTime, impactFlash, impactRadius       float64
	gameOver                                                  bool
	touchID                                                   ebiten.TouchID
	touchStart                                                vector
	touchActive                                               bool
	operationTicks                                            int
}

type saveData struct {
	PlayerX, PlayerY                float64
	Score, Wave, Scrap, TurretCount int
	SavedAt                         time.Time
}

func newGame() *game {
	game := &game{player: vector{screenW / 2, screenH / 2}, wave: 1, heldShard: -1}
	game.resetWave()
	return game
}
func (g *game) resetWave() {
	g.shards = make([]shard, shardCount)
	for index := range g.shards {
		g.shards[index] = shard{position: vector{80 + rand.Float64()*(worldW-160), 110 + rand.Float64()*(worldH-180)}, polarity: index % 2, active: true}
	}
	g.enemies = make([]enemy, 3+g.wave)
	for index := range g.enemies {
		health := 2 + index%3
		g.enemies[index] = enemy{position: vector{80 + rand.Float64()*(worldW-160), 120 + rand.Float64()*(worldH-180)}, health: health, maxHealth: health, kind: index % 3, active: true}
	}
	if len(g.resources) == 0 {
		g.resources = make([]resourceNode, 48)
		for index := range g.resources {
			centerX := 140 + rand.Float64()*(worldW-280)
			centerY := 160 + rand.Float64()*(worldH-320)
			if index > 5 {
				center := g.resources[rand.Intn(index)].position
				centerX = center.x + rand.NormFloat64()*170
				centerY = center.y + rand.NormFloat64()*130
			}
			centerX = math.Max(70, math.Min(worldW-70, centerX))
			centerY = math.Max(110, math.Min(worldH-110, centerY))
			g.resources[index] = resourceNode{position: vector{centerX, centerY}, amount: 3 + rand.Intn(5), active: true}
		}
	}
	if g.turrets == nil {
		g.turrets = []turret{}
	}
	g.projectiles = nil
	g.charge = 1
	g.combo = 0
	g.heldShard = -1
}
func (g *game) Update() error {
	if g.gameOver {
		if inpututil.IsKeyJustPressed(ebiten.KeyR) {
			*g = *newGame()
		}
		return nil
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF5) {
		g.save()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyF9) {
		g.load()
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyQ) {
		g.polarity = 0
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyE) {
		g.polarity = 1
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyB) {
		g.deployTurret()
	}
	g.movePlayer()
	mouseX, mouseY := ebiten.CursorPosition()
	cursor := vector{g.player.x + float64(mouseX-screenW/2), g.player.y + float64(mouseY-screenH/2)}
	g.updateTouchInput()
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		g.captureShard()
	}
	if g.heldShard >= 0 && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) {
		g.pullHeldShard()
	}
	if g.heldShard >= 0 && inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
		g.launchHeldShard(cursor)
	}
	if inpututil.IsKeyJustPressed(ebiten.KeySpace) && g.charge >= 1 {
		g.pulse = 1
		g.charge = 0
	}
	if g.pulse > 0 {
		g.pulse -= .055
	}
	if g.comboTime > 0 {
		g.comboTime -= 1
		if g.comboTime <= 0 {
			g.combo = 0
		}
	}
	if g.impactFlash > 0 {
		g.impactFlash -= .08
	}
	for index := range g.shards {
		g.updateShard(&g.shards[index])
	}
	for index := range g.enemies {
		g.updateEnemy(&g.enemies[index])
	}
	for index := range g.resources {
		node := &g.resources[index]
		if node.active && math.Hypot(node.position.x-g.player.x, node.position.y-g.player.y) < 28 {
			g.scrap += node.amount
			node.active = false
		}
	}
	for index := range g.turrets {
		g.updateTurret(&g.turrets[index])
	}
	for index := range g.projectiles {
		g.updateProjectile(&g.projectiles[index])
	}
	g.operationTicks++
	if g.operationTicks%60 == 0 {
		g.scrap += len(g.turrets)
		g.save()
	}
	remaining := 0
	for _, currentEnemy := range g.enemies {
		if currentEnemy.active {
			remaining++
		}
	}
	if remaining == 0 {
		g.wave++
		g.resetWave()
	}
	return nil
}

func (g *game) save() {
	data := saveData{g.player.x, g.player.y, g.score, g.wave, g.scrap, len(g.turrets), time.Now()}
	encoded, err := json.Marshal(data)
	if err == nil {
		_ = os.WriteFile("magnet_rush_save.json", encoded, 0644)
	}
}
func (g *game) load() {
	encoded, err := os.ReadFile("magnet_rush_save.json")
	if err != nil {
		return
	}
	var data saveData
	if json.Unmarshal(encoded, &data) == nil {
		g.player.x, g.player.y, g.score, g.wave, g.scrap = data.PlayerX, data.PlayerY, data.Score, data.Wave, data.Scrap
	}
}

func (g *game) updateTouchInput() {
	pressed := inpututil.AppendJustPressedTouchIDs(nil)
	for _, id := range pressed {
		x, y := ebiten.TouchPosition(id)
		if x < screenW/2 && !g.touchActive {
			g.touchID = id
			g.touchStart = vector{float64(x), float64(y)}
			g.touchActive = true
			continue
		}
		if x >= screenW/2 && g.heldShard < 0 {
			g.captureShard()
		}
	}
	if g.touchActive {
		x, y := ebiten.TouchPosition(g.touchID)
		dx, dy := float64(x)-g.touchStart.x, float64(y)-g.touchStart.y
		length := math.Hypot(dx, dy)
		if length > 12 {
			g.player.x += dx / length * 4.5
			g.player.y += dy / length * 4.5
			g.player.x = math.Max(24, math.Min(worldW-24, g.player.x))
			g.player.y = math.Max(96, math.Min(worldH-24, g.player.y))
		}
		if g.heldShard >= 0 {
			g.pullHeldShard()
		}
	}
	released := inpututil.AppendJustReleasedTouchIDs(nil)
	for _, id := range released {
		if id == g.touchID {
			g.touchActive = false
		}
		if g.heldShard >= 0 {
			x, y := inpututil.TouchPositionInPreviousTick(id)
			g.launchHeldShard(vector{g.player.x + float64(x-screenW/2), g.player.y + float64(y-screenH/2)})
		}
	}
}
func (g *game) deployTurret() {
	if g.scrap < 5 {
		return
	}
	g.scrap -= 5
	g.turrets = append(g.turrets, turret{position: g.player, active: true})
}
func (g *game) updateTurret(t *turret) {
	if t.cooldown > 0 {
		t.cooldown--
		return
	}
	closest, distance := -1, 170.0
	for index := range g.enemies {
		e := &g.enemies[index]
		if !e.active {
			continue
		}
		d := math.Hypot(e.position.x-t.position.x, e.position.y-t.position.y)
		if d < distance {
			closest, distance = index, d
		}
	}
	if closest >= 0 {
		e := g.enemies[closest]
		dx, dy := e.position.x-t.position.x, e.position.y-t.position.y
		n := math.Hypot(dx, dy)
		g.projectiles = append(g.projectiles, projectile{position: t.position, velocity: vector{dx / n * 8, dy / n * 8}, active: true})
		t.cooldown = 45
	}
}
func (g *game) updateProjectile(p *projectile) {
	if !p.active {
		return
	}
	p.position.x += p.velocity.x
	p.position.y += p.velocity.y
	if p.position.x < 0 || p.position.x > worldW || p.position.y < 0 || p.position.y > worldH {
		p.active = false
		return
	}
	for index := range g.enemies {
		e := &g.enemies[index]
		if e.active && math.Hypot(p.position.x-e.position.x, p.position.y-e.position.y) < 18 {
			e.health--
			e.hitFlash = 1
			p.active = false
			if e.health <= 0 {
				e.active = false
				g.score += 60
				g.charge = math.Min(1, g.charge+.04)
			}
			return
		}
	}
}
func (g *game) movePlayer() {
	x, y := 0., 0.
	if ebiten.IsKeyPressed(ebiten.KeyA) || ebiten.IsKeyPressed(ebiten.KeyLeft) {
		x--
	}
	if ebiten.IsKeyPressed(ebiten.KeyD) || ebiten.IsKeyPressed(ebiten.KeyRight) {
		x++
	}
	if ebiten.IsKeyPressed(ebiten.KeyW) || ebiten.IsKeyPressed(ebiten.KeyUp) {
		y--
	}
	if ebiten.IsKeyPressed(ebiten.KeyS) || ebiten.IsKeyPressed(ebiten.KeyDown) {
		y++
	}
	if x != 0 || y != 0 {
		length := math.Hypot(x, y)
		g.player.x += x / length * 4.5
		g.player.y += y / length * 4.5
	}
	g.player.x = math.Max(24, math.Min(worldW-24, g.player.x))
	g.player.y = math.Max(96, math.Min(worldH-24, g.player.y))
}
func (g *game) captureShard() {
	closest, distance := -1, magneticRange
	for index := range g.shards {
		current := &g.shards[index]
		if !current.active || current.polarity != g.polarity {
			continue
		}
		candidate := math.Hypot(current.position.x-g.player.x, current.position.y-g.player.y)
		if candidate < distance {
			closest, distance = index, candidate
		}
	}
	if closest >= 0 {
		g.heldShard = closest
		g.shards[closest].held = true
	}
}
func (g *game) pullHeldShard() {
	current := &g.shards[g.heldShard]
	x, y := g.player.x-current.position.x, g.player.y-current.position.y
	distance := math.Hypot(x, y)
	current.velocity.x += x / math.Max(distance, 1) * .72
	current.velocity.y += y / math.Max(distance, 1) * .72
}
func (g *game) launchHeldShard(cursor vector) {
	current := &g.shards[g.heldShard]
	x, y := cursor.x-g.player.x, cursor.y-g.player.y
	length := math.Hypot(x, y)
	if length > 0 {
		current.velocity = vector{x / length * 10.5, y / length * 10.5}
	}
	current.held = false
	g.heldShard = -1
}
func (g *game) updateShard(current *shard) {
	if !current.active {
		return
	}
	x, y := g.player.x-current.position.x, g.player.y-current.position.y
	distance := math.Hypot(x, y)
	if !current.held && distance < magneticRange {
		force := .06
		if current.polarity == g.polarity {
			force = .12
		} else {
			force = -.06
		}
		current.velocity.x += x / math.Max(distance, 1) * force
		current.velocity.y += y / math.Max(distance, 1) * force
	}
	if g.pulse > 0 {
		current.velocity.x -= x / math.Max(distance, 1) * 1.4
		current.velocity.y -= y / math.Max(distance, 1) * 1.4
	}
	current.velocity.x *= .986
	current.velocity.y *= .986
	current.position.x += current.velocity.x
	current.position.y += current.velocity.y
	if current.position.x < 18 || current.position.x > worldW-18 {
		current.velocity.x *= -1
	}
	if current.position.y < 90 || current.position.y > worldH-18 {
		current.velocity.y *= -1
	}
	for index := range g.enemies {
		target := &g.enemies[index]
		if target.active && math.Hypot(current.position.x-target.position.x, current.position.y-target.position.y) < 23 && math.Hypot(current.velocity.x, current.velocity.y) > 2.2 {
			target.active = false
			current.active = false
			current.held = false
			g.heldShard = -1
			g.combo++
			g.comboTime = 110
			g.score += 120 * (1 + g.combo)
			g.bestCombo = max(g.bestCombo, g.combo)
			g.charge = math.Min(1, g.charge+.18)
			g.impactFlash = 1
			g.impactRadius = 1
			return
		}
	}
}
func (g *game) updateEnemy(current *enemy) {
	if !current.active {
		return
	}
	if current.hitFlash > 0 {
		current.hitFlash -= .12
	}
	x, y := g.player.x-current.position.x, g.player.y-current.position.y
	distance := math.Hypot(x, y)
	if distance > 0 {
		speed := .42 + float64(g.wave)*.035
		if current.kind == 1 {
			speed *= 1.55
		}
		if current.kind == 2 {
			speed *= .7
		}
		current.position.x += x / distance * speed
		current.position.y += y / distance * speed
	}
	if distance < 22 {
		g.gameOver = true
	}
}
func (g *game) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{12, 18, 25, 255})
	camX := math.Max(0, math.Min(worldW-screenW, g.player.x-screenW/2))
	camY := math.Max(0, math.Min(worldH-screenH, g.player.y-screenH/2))
	for x := 0; x < screenW; x += 32 {
		ebitenutil.DrawRect(screen, float64(x), 72, 1, screenH-72, color.RGBA{30, 45, 52, 255})
	}
	for y := 72; y < screenH; y += 32 {
		ebitenutil.DrawRect(screen, 0, float64(y), screenW, 1, color.RGBA{30, 45, 52, 255})
	}
	pc := g.polarityColor()
	if g.impactRadius > 0 {
		g.impactRadius -= .055
	}
	for _, current := range g.shards {
		if !current.active {
			continue
		}
		c := color.RGBA{56, 211, 198, 255}
		if current.polarity == 1 {
			c = color.RGBA{255, 157, 72, 255}
		}
		if current.held {
			ebitenutil.DrawLine(screen, g.player.x-camX, g.player.y-camY, current.position.x-camX, current.position.y-camY, color.RGBA{c.R, c.G, c.B, 160})
		}
		ebitenutil.DrawRect(screen, current.position.x-camX-7, current.position.y-camY-7, 14, 14, c)
	}
	for _, current := range g.enemies {
		if current.active {
			enemyColor := color.RGBA{224, 67, 82, 255}
			if current.kind == 1 {
				enemyColor = color.RGBA{255, 142, 63, 255}
			}
			if current.kind == 2 {
				enemyColor = color.RGBA{176, 91, 229, 255}
			}
			if current.hitFlash > 0 {
				enemyColor = color.RGBA{255, 255, 255, 255}
			}
			size := 10.0
			if current.kind == 2 {
				size = 14
			}
			ebitenutil.DrawRect(screen, current.position.x-camX-size, current.position.y-camY-size, size*2, size*2, enemyColor)
			ebitenutil.DrawRect(screen, current.position.x-camX-5, current.position.y-camY-5, 10, 10, color.RGBA{255, 205, 195, 255})
			ebitenutil.DrawRect(screen, current.position.x-camX-12, current.position.y-camY-17, 24, 3, color.RGBA{50, 20, 24, 255})
			ebitenutil.DrawRect(screen, current.position.x-camX-12, current.position.y-camY-17, 24*float64(current.health)/float64(current.maxHealth), 3, color.RGBA{255, 100, 100, 255})
		}
	}
	for _, node := range g.resources {
		if node.active {
			ebitenutil.DrawRect(screen, node.position.x-camX-6, node.position.y-camY-6, 12, 12, color.RGBA{236, 214, 92, 255})
		}
	}
	for _, tower := range g.turrets {
		if tower.active {
			ebitenutil.DrawRect(screen, tower.position.x-camX-11, tower.position.y-camY-11, 22, 22, color.RGBA{152, 116, 255, 255})
			ebitenutil.DrawLine(screen, tower.position.x-camX-16, tower.position.y-camY, tower.position.x-camX+16, tower.position.y-camY, color.RGBA{205, 185, 255, 180})
			ebitenutil.DrawLine(screen, tower.position.x-camX, tower.position.y-camY-16, tower.position.x-camX, tower.position.y-camY+16, color.RGBA{205, 185, 255, 180})
		}
	}
	for _, shot := range g.projectiles {
		if shot.active {
			ebitenutil.DrawRect(screen, shot.position.x-camX-4, shot.position.y-camY-4, 8, 8, color.RGBA{255, 240, 170, 255})
		}
	}
	if g.impactRadius > 0 {
		radius := (1 - g.impactRadius) * 90
		ebitenutil.DrawRect(screen, g.player.x-camX-radius, g.player.y-camY-1, radius*2, 2, color.RGBA{255, 240, 170, 180})
		ebitenutil.DrawRect(screen, g.player.x-camX-1, g.player.y-camY-radius, 2, radius*2, color.RGBA{255, 240, 170, 180})
	}
	mouseX, mouseY := ebiten.CursorPosition()
	ebitenutil.DrawLine(screen, g.player.x-camX, g.player.y-camY, float64(mouseX), float64(mouseY), color.RGBA{pc.R, pc.G, pc.B, 110})
	if g.pulse > 0 {
		r := (1 - g.pulse) * 230
		ebitenutil.DrawRect(screen, g.player.x-r, g.player.y-1, r*2, 2, color.RGBA{255, 255, 255, 200})
		ebitenutil.DrawRect(screen, g.player.x-1, g.player.y-r, 2, r*2, color.RGBA{255, 255, 255, 200})
	}
	ebitenutil.DrawRect(screen, g.player.x-camX-14, g.player.y-camY-14, 28, 28, pc)
	ebitenutil.DrawRect(screen, g.player.x-camX-5, g.player.y-camY-5, 10, 10, color.RGBA{12, 18, 25, 255})
	ebitenutil.DrawRect(screen, 0, 0, screenW, 72, color.RGBA{8, 12, 17, 255})
	ebitenutil.DebugPrintAt(screen, "MAGNET RUSH  //  磁暴工坊", 20, 16)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("SCORE %06d   CHAIN x%d   BEST x%d   STORM %02d", g.score, g.combo, g.bestCombo, g.wave), 300, 16)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("SCRAP %02d  TURRETS %02d", g.scrap, len(g.turrets)), 700, 16)
	ebitenutil.DebugPrintAt(screen, "F5 SAVE  F9 LOAD  |  TURRETS PRODUCE SCRAP OVER TIME", 520, 34)
	ebitenutil.DebugPrintAt(screen, "RED HUNTER  ORANGE RUNNER  PURPLE TANK", 520, screenH-38)
	// 世界小地图：让玩家始终知道当前区域只是大地图的一部分。
	mapX, mapY, mapW, mapH := float64(screenW-180), 82.0, 150.0, 110.0
	ebitenutil.DrawRect(screen, mapX, mapY, mapW, mapH, color.RGBA{5, 9, 13, 220})
	for _, node := range g.resources {
		if node.active {
			ebitenutil.DrawRect(screen, mapX+node.position.x/worldW*mapW-2, mapY+node.position.y/worldH*mapH-2, 4, 4, color.RGBA{236, 214, 92, 255})
		}
	}
	for _, tower := range g.turrets {
		ebitenutil.DrawRect(screen, mapX+tower.position.x/worldW*mapW-2, mapY+tower.position.y/worldH*mapH-2, 4, 4, color.RGBA{152, 116, 255, 255})
	}
	for _, current := range g.enemies {
		if current.active {
			ebitenutil.DrawRect(screen, mapX+current.position.x/worldW*mapW-2, mapY+current.position.y/worldH*mapH-2, 4, 4, color.RGBA{224, 67, 82, 255})
		}
	}
	ebitenutil.DrawRect(screen, mapX+g.player.x/worldW*mapW-3, mapY+g.player.y/worldH*mapH-3, 6, 6, pc)
	viewW, viewH := screenW/worldW*mapW, screenH/worldH*mapH
	ebitenutil.DrawRect(screen, mapX+camX/worldW*mapW, mapY+camY/worldH*mapH, viewW, 1, color.RGBA{180, 200, 210, 180})
	ebitenutil.DrawRect(screen, mapX+camX/worldW*mapW, mapY+(camY+screenH)/worldH*mapH, viewW, 1, color.RGBA{180, 200, 210, 180})
	ebitenutil.DrawRect(screen, mapX+camX/worldW*mapW, mapY+camY/worldH*mapH, 1, viewH, color.RGBA{180, 200, 210, 180})
	ebitenutil.DrawRect(screen, mapX+(camX+screenW)/worldW*mapW, mapY+camY/worldH*mapH, 1, viewH, color.RGBA{180, 200, 210, 180})
	ebitenutil.DebugPrintAt(screen, "SECTOR MAP", int(mapX)+48, int(mapY+mapH+5))
	ebitenutil.DrawRect(screen, 20, 48, 150, 8, color.RGBA{50, 50, 50, 255})
	ebitenutil.DrawRect(screen, 20, 48, 150*g.charge, 8, pc)
	ebitenutil.DebugPrintAt(screen, "SPACE  OVERLOAD", 185, 46)
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("SECTOR OBJECTIVE: SURVIVE STORM %02d  |  HOLD LMB CAPTURE -> RELEASE TO LAUNCH", g.wave), 20, screenH-38)
	ebitenutil.DebugPrintAt(screen, "WASD MOVE  Q/E POLARITY  B DEPLOY TURRET (5 SCRAP)  LMB CAPTURE/LAUNCH", 20, screenH-20)
	// 触摸提示：左侧拖动移动，右侧触摸并拖动磁力操作。
	ebitenutil.DrawRect(screen, 28, screenH-150, 96, 96, color.RGBA{40, 70, 80, 90})
	ebitenutil.DrawRect(screen, screenW-124, screenH-150, 96, 96, color.RGBA{80, 58, 42, 90})
	ebitenutil.DebugPrintAt(screen, "MOVE", 58, screenH-104)
	ebitenutil.DebugPrintAt(screen, "MAGNET", screenW-108, screenH-104)
	if g.impactFlash > 0 {
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("MAGNETIC KILL!  +%d  CHAIN x%d", 120*(1+g.combo), g.combo), screenW/2-110, 100)
	}
	if g.gameOver {
		ebitenutil.DrawRect(screen, 250, 220, 460, 150, color.RGBA{8, 12, 17, 240})
		ebitenutil.DebugPrintAt(screen, "CORE COLLAPSED", 400, 255)
		ebitenutil.DebugPrintAt(screen, fmt.Sprintf("SCORE %d  BEST CHAIN x%d", g.score, g.bestCombo), 370, 285)
		ebitenutil.DebugPrintAt(screen, "PRESS R TO RESTART", 390, 325)
	}
}
func (g *game) polarityColor() color.RGBA {
	if g.polarity == 1 {
		return color.RGBA{255, 157, 72, 255}
	}
	return color.RGBA{56, 211, 198, 255}
}
func (g *game) Layout(_, _ int) (int, int) { return screenW, screenH }
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func main() {
	ebiten.SetWindowSize(screenW, screenH)
	ebiten.SetWindowTitle("磁暴工坊 · Magnet Rush")
	if err := ebiten.RunGame(newGame()); err != nil {
		panic(err)
	}
}

// mautrix-slack - A Matrix-Slack puppeting bridge.
// Copyright (C) 2022 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package database

import (
	"database/sql"
	"sync"

	log "maunium.net/go/maulogger/v2"

	"maunium.net/go/mautrix/id"
	"maunium.net/go/mautrix/util/dbutil"
)

type User struct {
	db  *Database
	log log.Logger

	MXID           id.UserID
	ManagementRoom id.RoomID
	SpaceRoom      id.RoomID

	TeamsLock sync.Mutex
	Teams     map[string]*UserTeam
}

func (user *User) loadTeams() {
	user.TeamsLock.Lock()
	defer user.TeamsLock.Unlock()

	for _, userTeam := range user.db.UserTeam.GetAllByMXIDWithToken(user.MXID) {
		user.Teams[userTeam.Key.TeamID] = userTeam
	}
}

func (u *User) Scan(row dbutil.Scannable) *User {
	var spaceRoom sql.NullString

	err := row.Scan(&u.MXID, &u.ManagementRoom, &spaceRoom)
	if err != nil {
		if err != sql.ErrNoRows {
			u.log.Errorln("Database scan failed:", err)
		}

		return nil
	}

	u.SpaceRoom = id.RoomID(spaceRoom.String)

	u.loadTeams()

	return u
}

func (u *User) SyncTeams() {
	u.TeamsLock.Lock()
	defer u.TeamsLock.Unlock()

	for _, userteam := range u.Teams {
		userteam.Upsert()
	}

	// Delete not logged in teams from the database.
	query := "DELETE FROM user_team WHERE mxid=$1 AND token=NULL"

	_, err := u.db.Exec(query, u.MXID)
	if err != nil {
		u.log.Warnfln("Failed to prune old teams for %s: %v", u.MXID, err)
	}
}

func (u *User) Insert() {
	query := "INSERT INTO \"user\" (mxid, management_room, space_room) VALUES ($1, $2, $3);"

	_, err := u.db.Exec(query, u.MXID, u.ManagementRoom, u.SpaceRoom)

	if err != nil {
		u.log.Warnfln("Failed to insert %s: %v", u.MXID, err)
	}

	u.SyncTeams()
}

func (u *User) Update() {
	query := "UPDATE \"user\" SET management_room=$1, space_room=$2 WHERE mxid=$3;"

	_, err := u.db.Exec(query, u.ManagementRoom, u.SpaceRoom, u.MXID)

	if err != nil {
		u.log.Warnfln("Failed to update %q: %v", u.MXID, err)
	}

	u.SyncTeams()
}

func (u *User) TeamLoggedIn(email, domain string) bool {
	u.TeamsLock.Lock()
	defer u.TeamsLock.Unlock()

	for _, team := range u.Teams {
		if team.SlackEmail == email && team.TeamName == domain {
			return true
		}
	}

	return false
}

func (u *User) GetLoggedInTeams() []*UserTeam {
	u.TeamsLock.Lock()
	defer u.TeamsLock.Unlock()

	teams := []*UserTeam{}

	for _, team := range u.Teams {
		if team.Token != "" {
			teams = append(teams, team)
		}
	}

	return teams
}

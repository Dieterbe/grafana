package sqlstore

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana/pkg/bus"
	"github.com/grafana/grafana/pkg/events"
	"github.com/grafana/grafana/pkg/log"
	m "github.com/grafana/grafana/pkg/models"
)

func init() {
	bus.AddHandler("sql", GetMonitors)
	bus.AddHandler("sql", GetMonitorById)
	bus.AddHandler("sql", GetMonitorTypes)
	bus.AddHandler("sql", AddMonitor)
	bus.AddHandler("sql", UpdateMonitor)
	bus.AddHandler("sql", DeleteMonitor)
}

type MonitorWithCollectorDTO struct {
	Id            int64
	EndpointId    int64
	OrgId         int64
	Namespace     string
	MonitorTypeId int64
	CollectorIds  string
	CollectorTags string
	TagCollectors string
	Settings      []*m.MonitorSettingDTO
	Frequency     int64
	Enabled       bool
	Offset        int64
	Updated       time.Time
	Created       time.Time
}

func GetMonitorById(query *m.GetMonitorByIdQuery) error {
	sess := x.Table("monitor")
	rawParams := make([]interface{}, 0)
	rawSql := `
SELECT
    GROUP_CONCAT(DISTINCT(monitor_collector.collector_id)) as collector_ids,
    GROUP_CONCAT(DISTINCT(monitor_collector_tag.tag)) as collector_tags,
    GROUP_CONCAT(DISTINCT(collector_tags.collector_id)) as tag_collectors,
    monitor.*
FROM monitor
    LEFT JOIN monitor_collector ON monitor.id = monitor_collector.monitor_id
    LEFT JOIN monitor_collector_tag ON monitor.id = monitor_collector_tag.monitor_id
    LEFT JOIN 
        (SELECT
            collector.id AS collector_id,
            collector_tag.tag as tag
        FROM collector
        LEFT JOIN collector_tag ON collector.id = collector_tag.collector_id
        WHERE (collector.public=1 OR collector.org_id=?) AND (collector_tag.org_id=? OR collector_tag.id is NULL)) as collector_tags
    ON collector_tags.tag = monitor_collector_tag.tag
WHERE monitor.id=?
	`
	rawParams = append(rawParams, query.OrgId, query.OrgId, query.Id)

	if !query.IsGrafanaAdmin {
		rawSql += "AND monitor.org_id=?\n"
		rawParams = append(rawParams, query.OrgId)
	}
	rawSql += "GROUP BY monitor.id"

	//store the results into an array of maps.
	results := make([]*MonitorWithCollectorDTO, 0)
	err := sess.Sql(rawSql, rawParams...).Find(&results)
	if err != nil {
		return err
	}
	result := results[0]

	monitorCollectorIds := make([]int64, 0)
	monitorCollectorsMap := make(map[int64]bool)
	if result.CollectorIds != "" {
		for _, l := range strings.Split(result.CollectorIds, ",") {
			i, err := strconv.ParseInt(l, 10, 64)
			if err != nil {
				return err
			}
			monitorCollectorIds = append(monitorCollectorIds, i)
			monitorCollectorsMap[i] = true
		}
	}

	monitorCollectorTags := make([]string, 0)
	if result.CollectorTags != "" {
		monitorCollectorTags = strings.Split(result.CollectorTags, ",")
		for _, l := range strings.Split(result.TagCollectors, ",") {
			i, err := strconv.ParseInt(l, 10, 64)
			if err != nil {
				return err
			}
			monitorCollectorsMap[i] = true
		}
	}

	mergedCollectors := make([]int64, len(monitorCollectorsMap))
	count := 0
	for k := range monitorCollectorsMap {
		mergedCollectors[count] = k
		count += 1
	}

	query.Result = &m.MonitorDTO{
		Id:            result.Id,
		EndpointId:    result.EndpointId,
		OrgId:         result.OrgId,
		Namespace:     result.Namespace,
		MonitorTypeId: result.MonitorTypeId,
		CollectorIds:  monitorCollectorIds,
		CollectorTags: monitorCollectorTags,
		Collectors:    mergedCollectors,
		Settings:      result.Settings,
		Frequency:     result.Frequency,
		Enabled:       result.Enabled,
		Offset:        result.Offset,
		Updated:       result.Updated,
	}

	return nil
}

func GetMonitors(query *m.GetMonitorsQuery) error {
	sess := x.Table("monitor")
	rawParams := make([]interface{}, 0)
	rawSql := `
SELECT
    GROUP_CONCAT(DISTINCT(monitor_collector.collector_id)) as collector_ids,
    GROUP_CONCAT(DISTINCT(monitor_collector_tag.tag)) as collector_tags,
    GROUP_CONCAT(DISTINCT(collector_tags.collector_id)) as tag_collectors,
    monitor.*
FROM monitor
    LEFT JOIN monitor_collector ON monitor.id = monitor_collector.monitor_id
    LEFT JOIN monitor_collector_tag ON monitor.id = monitor_collector_tag.monitor_id
    LEFT JOIN 
        (SELECT
            collector.id AS collector_id,
            collector_tag.tag as tag
        FROM collector
        LEFT JOIN collector_tag ON collector.id = collector_tag.collector_id
        WHERE (collector.public=1 OR collector.org_id=?) AND (collector_tag.org_id=? OR collector_tag.id is NULL)) as collector_tags
    ON collector_tags.tag = monitor_collector_tag.tag
`
	rawParams = append(rawParams, query.OrgId, query.OrgId)
	whereSql := make([]string, 0)
	if !query.IsGrafanaAdmin {
		whereSql = append(whereSql, "monitor.org_id=?")
		rawParams = append(rawParams, query.OrgId)
	}

	if len(query.EndpointId) > 0 {
		p := make([]string, len(query.EndpointId))
		for i, e := range query.EndpointId {
			p[i] = "?"
			rawParams = append(rawParams, e)
		}
		whereSql = append(whereSql, fmt.Sprintf("monitor.endpoint_id IN (%s)", strings.Join(p, ",")))
	}

	if query.Modulo > 0 {
		whereSql = append(whereSql, "(monitor.id % ?) = ?")
		rawParams = append(rawParams, query.Modulo, query.ModuloOffset)
	}

	if len(whereSql) > 0 {
		rawSql += "WHERE " + strings.Join(whereSql, " AND ")
	}
	rawSql += " GROUP BY monitor.id"

	result := make([]*MonitorWithCollectorDTO, 0)
	err := sess.Sql(rawSql, rawParams...).Find(&result)
	if err != nil {
		return err
	}

	monitors := make([]*m.MonitorDTO, 0)
	//iterate through all of the results and build out our checks model.
	for _, row := range result {
		monitorCollectorIds := make([]int64, 0)
		monitorCollectorsMap := make(map[int64]bool)
		if row.CollectorIds != "" {
			for _, l := range strings.Split(row.CollectorIds, ",") {
				i, err := strconv.ParseInt(l, 10, 64)
				if err != nil {
					return err
				}
				monitorCollectorIds = append(monitorCollectorIds, i)
				monitorCollectorsMap[i] = true
			}
		}

		monitorCollectorTags := make([]string, 0)
		if row.CollectorTags != "" {
			monitorCollectorTags = strings.Split(row.CollectorTags, ",")
			for _, l := range strings.Split(row.TagCollectors, ",") {
				i, err := strconv.ParseInt(l, 10, 64)
				if err != nil {
					return err
				}
				monitorCollectorsMap[i] = true
			}
		}

		mergedCollectors := make([]int64, len(monitorCollectorsMap))
		count := 0
		for k := range monitorCollectorsMap {
			mergedCollectors[count] = k
			count += 1
		}

		monitors = append(monitors, &m.MonitorDTO{
			Id:            row.Id,
			EndpointId:    row.EndpointId,
			OrgId:         row.OrgId,
			Namespace:     row.Namespace,
			MonitorTypeId: row.MonitorTypeId,
			CollectorIds:  monitorCollectorIds,
			CollectorTags: monitorCollectorTags,
			Collectors:    mergedCollectors,
			Settings:      row.Settings,
			Frequency:     row.Frequency,
			Enabled:       row.Enabled,
			Offset:        row.Offset,
			Updated:       row.Updated,
		})
	}
	query.Result = monitors

	return nil

}

type MonitorTypeWithSettingsDTO struct {
	Id           int64
	Name         string
	Variable     string
	Description  string
	Required     bool
	DataType     string
	Conditions   map[string]interface{}
	DefaultValue string
}

func GetMonitorTypes(query *m.GetMonitorTypesQuery) error {
	sess := x.Table("monitor_type")
	sess.Limit(100, 0).Asc("name")
	sess.Join("LEFT", "monitor_type_setting", "monitor_type_setting.monitor_type_id=monitor_type.id")
	sess.Cols("monitor_type.id", "monitor_type.name",
		"monitor_type_setting.variable", "monitor_type_setting.description",
		"monitor_type_setting.required", "monitor_type_setting.data_type",
		"monitor_type_setting.conditions", "monitor_type_setting.default_value")

	result := make([]*MonitorTypeWithSettingsDTO, 0)
	err := sess.Find(&result)
	if err != nil {
		return err
	}
	types := make(map[int64]*m.MonitorTypeDTO)
	//iterate through all of the results and build out our checks model.
	for _, row := range result {
		if _, ok := types[row.Id]; ok != true {
			//this is the first time we have seen this monitorId
			var typeSettings []*m.MonitorTypeSettingDTO
			types[row.Id] = &m.MonitorTypeDTO{
				Id:       row.Id,
				Name:     row.Name,
				Settings: typeSettings,
			}
		}

		types[row.Id].Settings = append(types[row.Id].Settings, &m.MonitorTypeSettingDTO{
			Variable:     row.Variable,
			Description:  row.Description,
			Required:     row.Required,
			DataType:     row.DataType,
			Conditions:   row.Conditions,
			DefaultValue: row.DefaultValue,
		})
	}

	query.Result = make([]*m.MonitorTypeDTO, len(types))
	count := 0
	for _, v := range types {
		query.Result[count] = v
		count++
	}

	return nil
}

func DeleteMonitor(cmd *m.DeleteMonitorCommand) error {
	return inTransaction2(func(sess *session) error {
		q := m.GetMonitorByIdQuery{
			Id:    cmd.Id,
			OrgId: cmd.OrgId,
		}
		err := GetMonitorById(&q)
		if err != nil {
			return err
		}
		var rawSql = "DELETE FROM monitor WHERE id=? and org_id=?"
		_, err = sess.Exec(rawSql, cmd.Id, cmd.OrgId)
		if err != nil {
			return err
		}
		rawSql = "DELETE FROM monitor_collector WHERE monitor_id=?"
		_, err = sess.Exec(rawSql, cmd.Id)
		if err != nil {
			return err
		}
		rawSql = "DELETE FROM monitor_collector_tag WHERE monitor_id=?"
		_, err = sess.Exec(rawSql, cmd.Id)
		if err != nil {
			return err
		}

		sess.publishAfterCommit(&events.MonitorRemoved{
			Timestamp:     time.Now(),
			Id:            q.Result.Id,
			EndpointId:    q.Result.EndpointId,
			OrgId:         q.Result.OrgId,
			CollectorIds:  q.Result.CollectorIds,
			CollectorTags: q.Result.CollectorTags,
		})
		return nil
	})
}

// store collector list query result
type collectorList struct {
	Id int64
}

func AddMonitor(cmd *m.AddMonitorCommand) error {

	return inTransaction2(func(sess *session) error {
		//validate Endpoint.
		endpointQuery := m.GetEndpointByIdQuery{
			Id:    cmd.EndpointId,
			OrgId: cmd.OrgId,
		}
		err := GetEndpointById(&endpointQuery)
		if err != nil {
			return err
		}

		filtered_collectors := make([]*collectorList, 0, len(cmd.CollectorIds))
		if len(cmd.CollectorIds) > 0 {
			sess.Table("collector")
			sess.In("id", cmd.CollectorIds).Where("org_id=? or public=1", cmd.OrgId)
			sess.Cols("id")
			err = sess.Find(&filtered_collectors)

			if err != nil {
				return err
			}
		}

		if len(filtered_collectors) < len(cmd.CollectorIds) {
			return m.ErrMonitorCollectorsInvalid
		}

		//get settings definition for thie monitorType.
		var typeSettings []*m.MonitorTypeSetting
		sess.Table("monitor_type_setting")
		sess.Where("monitor_type_id=?", cmd.MonitorTypeId)
		err = sess.Find(&typeSettings)
		if err != nil {
			return nil
		}

		// push the typeSettings into a Map with the variable name as key
		settingMap := make(map[string]*m.MonitorTypeSetting)
		for _, s := range typeSettings {
			settingMap[s.Variable] = s
		}

		//validate the settings.
		seenMetrics := make(map[string]bool)
		for _, v := range cmd.Settings {
			def, ok := settingMap[v.Variable]
			if ok != true {
				log.Info("Unkown variable %s passed.", v.Variable)
				return m.ErrMonitorSettingsInvalid
			}
			//TODO:(awoods) make sure the value meets the definition.
			seenMetrics[def.Variable] = true
			log.Info("%s present in settings", def.Variable)
		}

		//make sure all required variables were provided.
		//add defaults for missing optional variables.
		for k, s := range settingMap {
			if _, ok := seenMetrics[k]; ok != true {
				log.Info("%s not in settings", k)
				if s.Required {
					// required setting variable missing.
					return m.ErrMonitorSettingsInvalid
				}
				cmd.Settings = append(cmd.Settings, &m.MonitorSettingDTO{
					Variable: k,
					Value:    s.DefaultValue,
				})
			}
		}

		if cmd.Namespace == "" {
			label := strings.ToLower(endpointQuery.Result.Name)
			re := regexp.MustCompile("[^\\w-]+")
			re2 := regexp.MustCompile("\\s")
			slug := re2.ReplaceAllString(re.ReplaceAllString(label, "_"), "-")
			cmd.Namespace = slug
		}

		mon := &m.Monitor{
			EndpointId:    cmd.EndpointId,
			OrgId:         cmd.OrgId,
			Namespace:     cmd.Namespace,
			MonitorTypeId: cmd.MonitorTypeId,
			Offset:        rand.Int63n(cmd.Frequency - 1),
			Settings:      cmd.Settings,
			Created:       time.Now(),
			Updated:       time.Now(),
			Frequency:     cmd.Frequency,
			Enabled:       cmd.Enabled,
		}

		if _, err := sess.Insert(mon); err != nil {
			return err
		}

		if len(cmd.CollectorIds) > 0 {
			monitor_collectors := make([]*m.MonitorCollector, len(cmd.CollectorIds))
			for i, l := range cmd.CollectorIds {
				monitor_collectors[i] = &m.MonitorCollector{
					MonitorId:   mon.Id,
					CollectorId: l,
				}
			}
			sess.Table("monitor_collector")
			if _, err := sess.Insert(&monitor_collectors); err != nil {
				return err
			}
		}

		if len(cmd.CollectorTags) > 0 {
			monitor_collector_tags := make([]*m.MonitorCollectorTag, len(cmd.CollectorTags))
			for i, t := range cmd.CollectorTags {
				monitor_collector_tags[i] = &m.MonitorCollectorTag{
					MonitorId: mon.Id,
					Tag:       t,
				}
			}

			sess.Table("monitor_collector_tag")
			if _, err := sess.Insert(&monitor_collector_tags); err != nil {
				return err
			}
		}
		// get collectorIds from tags
		tagCollectors, err := getCollectorIdsFromTags(cmd.OrgId, cmd.CollectorTags, sess)
		if err != nil {
			return err
		}

		collectorIdMap := make(map[int64]bool)
		collectorList := make([]int64, 0)
		for _, id := range cmd.CollectorIds {
			collectorIdMap[id] = true
			collectorList = append(collectorList, id)
		}

		for _, id := range tagCollectors {
			if _, ok := collectorIdMap[id]; !ok {
				collectorList = append(collectorList, id)
			}
		}
		cmd.Result = &m.MonitorDTO{
			Id:            mon.Id,
			EndpointId:    mon.EndpointId,
			OrgId:         mon.OrgId,
			Namespace:     mon.Namespace,
			MonitorTypeId: mon.MonitorTypeId,
			CollectorIds:  cmd.CollectorIds,
			CollectorTags: cmd.CollectorTags,
			Collectors:    collectorList,
			Settings:      mon.Settings,
			Frequency:     mon.Frequency,
			Enabled:       mon.Enabled,
			Offset:        mon.Offset,
			Updated:       mon.Updated,
		}
		sess.publishAfterCommit(&events.MonitorCreated{
			Timestamp: mon.Updated,
			MonitorPayload: events.MonitorPayload{
				Id:            mon.Id,
				EndpointId:    mon.EndpointId,
				OrgId:         mon.OrgId,
				Namespace:     mon.Namespace,
				MonitorTypeId: mon.MonitorTypeId,
				CollectorIds:  cmd.CollectorIds,
				CollectorTags: cmd.CollectorTags,
				Collectors:    collectorList,
				Settings:      mon.Settings,
				Frequency:     mon.Frequency,
				Enabled:       mon.Enabled,
				Offset:        mon.Offset,
				Updated:       mon.Updated,
			},
		})
		return nil
	})
}

func UpdateMonitor(cmd *m.UpdateMonitorCommand) error {
	return inTransaction2(func(sess *session) error {
		//validate Endpoint.
		endpointQuery := m.GetEndpointByIdQuery{
			Id:    cmd.EndpointId,
			OrgId: cmd.OrgId,
		}
		err := GetEndpointById(&endpointQuery)
		if err != nil {
			return err
		}
		currentEndpoint := endpointQuery.Result

		q := m.GetMonitorByIdQuery{
			Id:    cmd.Id,
			OrgId: cmd.OrgId,
		}
		err = GetMonitorById(&q)
		if err != nil {
			return err
		}
		lastState := q.Result

		if lastState.EndpointId != cmd.EndpointId {
			return m.ErrorEndpointCantBeChanged
		}

		//validate collectors.
		filtered_collectors := make([]*collectorList, 0, len(cmd.CollectorIds))
		if len(cmd.CollectorIds) > 0 {
			sess.Table("collector")
			sess.In("id", cmd.CollectorIds).Where("org_id=? or public=1", cmd.OrgId)
			sess.Cols("id")
			err = sess.Find(&filtered_collectors)

			if err != nil {
				return err
			}
		}

		if len(filtered_collectors) < len(cmd.CollectorIds) {
			return m.ErrMonitorCollectorsInvalid
		}

		//get settings definition for thie monitorType.
		var typeSettings []*m.MonitorTypeSetting
		sess.Table("monitor_type_setting")
		sess.Where("monitor_type_id=?", cmd.MonitorTypeId)
		err = sess.Find(&typeSettings)
		if err != nil {
			return nil
		}
		if len(typeSettings) < 1 {
			log.Info("no monitorType settings found for type: %d", cmd.MonitorTypeId)
			return m.ErrMonitorSettingsInvalid
		}

		// push the typeSettings into a Map with the variable name as key
		settingMap := make(map[string]*m.MonitorTypeSetting)
		for _, s := range typeSettings {
			settingMap[s.Variable] = s
		}

		//validate the settings.
		seenMetrics := make(map[string]bool)
		for _, v := range cmd.Settings {
			def, ok := settingMap[v.Variable]
			if ok != true {
				log.Info("Unkown variable %s passed.", v.Variable)
				return m.ErrMonitorSettingsInvalid
			}
			//TODO:(awoods) make sure the value meets the definition.
			seenMetrics[def.Variable] = true
		}

		//make sure all required variables were provided.
		//add defaults for missing optional variables.
		for k, s := range settingMap {
			if _, ok := seenMetrics[k]; ok != true {
				log.Info("%s not in settings", k)
				if s.Required {
					// required setting variable missing.
					return m.ErrMonitorSettingsInvalid
				}
				cmd.Settings = append(cmd.Settings, &m.MonitorSettingDTO{
					Variable: k,
					Value:    s.DefaultValue,
				})
			}
		}

		if cmd.Namespace == "" {
			label := strings.ToLower(currentEndpoint.Name)
			re := regexp.MustCompile("[^\\w-]+")
			re2 := regexp.MustCompile("\\s")
			slug := re2.ReplaceAllString(re.ReplaceAllString(label, "_"), "-")
			cmd.Namespace = slug
		}

		mon := &m.Monitor{
			Id:            cmd.Id,
			EndpointId:    cmd.EndpointId,
			OrgId:         cmd.OrgId,
			Namespace:     cmd.Namespace,
			MonitorTypeId: cmd.MonitorTypeId,
			Settings:      cmd.Settings,
			Updated:       time.Now(),
			Enabled:       cmd.Enabled,
			Frequency:     cmd.Frequency,
		}

		//check if we need to update the time offset for when the monitor should run.
		if mon.Offset >= mon.Frequency {
			mon.Offset = rand.Int63n(mon.Frequency - 1)
		}

		sess.UseBool("enabled")
		if _, err = sess.Where("id=? and org_id=?", mon.Id, mon.OrgId).Update(mon); err != nil {
			return err
		}

		var rawSql = "DELETE FROM monitor_collector WHERE monitor_id=?"
		if _, err := sess.Exec(rawSql, cmd.Id); err != nil {
			return err
		}
		if len(cmd.CollectorIds) > 0 {
			monitor_collectors := make([]*m.MonitorCollector, len(cmd.CollectorIds))
			for i, l := range cmd.CollectorIds {
				monitor_collectors[i] = &m.MonitorCollector{
					MonitorId:   cmd.Id,
					CollectorId: l,
				}
			}
			sess.Table("monitor_collector")
			if _, err := sess.Insert(&monitor_collectors); err != nil {
				return err
			}
		}

		rawSql = "DELETE FROM monitor_collector_tag WHERE monitor_id=?"
		if _, err := sess.Exec(rawSql, cmd.Id); err != nil {
			return err
		}
		if len(cmd.CollectorTags) > 0 {
			monitor_collector_tags := make([]*m.MonitorCollectorTag, len(cmd.CollectorTags))
			for i, t := range cmd.CollectorTags {
				monitor_collector_tags[i] = &m.MonitorCollectorTag{
					MonitorId: cmd.Id,
					Tag:       t,
				}
			}

			sess.Table("monitor_collector_tag")
			if _, err := sess.Insert(&monitor_collector_tags); err != nil {
				return err
			}
		}

		// get collectorIds from tags
		tagCollectors, err := getCollectorIdsFromTags(cmd.OrgId, cmd.CollectorTags, sess)
		if err != nil {
			return err
		}

		collectorIdMap := make(map[int64]bool)
		collectorList := make([]int64, 0)
		for _, id := range cmd.CollectorIds {
			collectorIdMap[id] = true
			collectorList = append(collectorList, id)
		}

		for _, id := range tagCollectors {
			if _, ok := collectorIdMap[id]; !ok {
				collectorList = append(collectorList, id)
			}
		}

		sess.publishAfterCommit(&events.MonitorUpdated{
			MonitorPayload: events.MonitorPayload{
				Id:            mon.Id,
				EndpointId:    mon.EndpointId,
				OrgId:         mon.OrgId,
				Namespace:     mon.Namespace,
				MonitorTypeId: mon.MonitorTypeId,
				CollectorIds:  cmd.CollectorIds,
				CollectorTags: cmd.CollectorTags,
				Collectors:    collectorList,
				Settings:      mon.Settings,
				Frequency:     mon.Frequency,
				Enabled:       mon.Enabled,
				Offset:        mon.Offset,
				Updated:       mon.Updated,
			},
			Timestamp: mon.Updated,
			LastState: &events.MonitorPayload{
				Id:            lastState.Id,
				EndpointId:    lastState.EndpointId,
				OrgId:         lastState.OrgId,
				Namespace:     lastState.Namespace,
				MonitorTypeId: lastState.MonitorTypeId,
				CollectorIds:  lastState.CollectorIds,
				CollectorTags: lastState.CollectorTags,
				Collectors:    lastState.Collectors,
				Settings:      lastState.Settings,
				Frequency:     lastState.Frequency,
				Enabled:       lastState.Enabled,
				Offset:        lastState.Offset,
				Updated:       lastState.Updated,
			},
		})

		return err
	})
}

type CollectorId struct {
	CollectorId int64
}

func getCollectorIdsFromTags(orgId int64, tags []string, sess *session) ([]int64, error) {
	result := make([]int64, 0)
	if len(tags) < 1 {
		return result, nil
	}
	params := make([]interface{}, 0)
	rawSql := `SELECT DISTINCT(collector.id) AS collector_id
	FROM collector
	INNER JOIN collector_tag ON collector.id = collector_tag.collector_id 
	WHERE (collector.public=1 OR collector.org_id=?) 
		AND collector_tag.org_id=?
	`

	params = append(params, orgId, orgId)

	p := make([]string, len(tags))
	for i, t := range tags {
		p[i] = "?"
		params = append(params, t)
	}
	rawSql += fmt.Sprintf("AND collector_tag.tag IN (%s)", strings.Join(p, ","))

	results := make([]CollectorId, 0)
	if err := sess.Sql(rawSql, params...).Find(&results); err != nil {
		return result, err
	}

	if len(results) > 0 {
		for _, r := range results {
			result = append(result, r.CollectorId)
		}
	}

	return result, nil
}
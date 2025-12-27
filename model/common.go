package model

import (
	"done-hub/common"
	"done-hub/common/config"
	"fmt"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"
)

type modelable interface {
	any
}

type GenericParams struct {
	PaginationParams
	Keyword string `form:"keyword"`
}

type PaginationParams struct {
	Page  int    `form:"page"`
	Size  int    `form:"size"`
	Order string `form:"order"`
}

type DataResult[T modelable] struct {
	Data       *[]*T `json:"data"`
	Page       int   `json:"page"`
	Size       int   `json:"size"`
	TotalCount int64 `json:"total_count"`
}

func PaginateAndOrder[T modelable](db *gorm.DB, params *PaginationParams, result *[]*T, allowedOrderFields map[string]bool) (*DataResult[T], error) {
	// 获取总数
	var totalCount int64
	err := db.Model(result).Count(&totalCount).Error
	if err != nil {
		return nil, err
	}

	// 分页
	if params.Page < 1 {
		params.Page = 1
	}
	if params.Size < 1 {
		params.Size = config.ItemsPerPage
	}

	if params.Size > config.MaxRecentItems {
		return nil, fmt.Errorf("size 参数不能超过 %d", config.MaxRecentItems)
	}

	offset := (params.Page - 1) * params.Size
	db = db.Offset(offset).Limit(params.Size)

	// 排序
	if params.Order != "" {
		orderFields := strings.Split(params.Order, ",")
		for _, field := range orderFields {
			field = strings.TrimSpace(field)
			desc := strings.HasPrefix(field, "-")
			if desc {
				field = field[1:]
			}
			if !allowedOrderFields[field] {
				return nil, fmt.Errorf("不允许对字段 '%s' 进行排序", field)
			}
			if desc {
				field = field + " DESC"
			}
			db = db.Order(field)
		}
	} else {
		// 默认排序
		db = db.Order("id DESC")
	}

	// 查询
	err = db.Find(result).Error
	if err != nil {
		return nil, err
	}

	// 返回结果
	return &DataResult[T]{
		Data:       result,
		Page:       params.Page,
		Size:       params.Size,
		TotalCount: totalCount,
	}, nil
}

func getDateFormat(groupType string) string {
	var dateFormat string
	if groupType == "day" {
		dateFormat = "%Y-%m-%d"
		if common.UsingPostgreSQL {
			dateFormat = "YYYY-MM-DD"
		}
	} else {
		dateFormat = "%Y-%m"
		if common.UsingPostgreSQL {
			dateFormat = "YYYY-MM"
		}
	}
	return dateFormat
}

func getTimestampGroupsSelect(fieldName, groupType, alias string) string {
	dateFormat := getDateFormat(groupType)
	var groupSelect string

	// 获取系统时区信息
	location := time.Local
	if tzEnv := os.Getenv("TZ"); tzEnv != "" {
		if loc, err := time.LoadLocation(tzEnv); err == nil {
			location = loc
		}
	}
	now := time.Now().In(location)
	_, offsetSeconds := now.Zone()

	// SQLite 需要特殊格式的偏移字符串
	getSqliteOffset := func() string {
		hours := offsetSeconds / 3600
		minutes := (offsetSeconds % 3600) / 60
		if hours >= 0 {
			offset := fmt.Sprintf("+%d hours", hours)
			if minutes != 0 {
				offset += fmt.Sprintf(" %d minutes", minutes)
			}
			return offset
		}
		offset := fmt.Sprintf("%d hours", hours)
		if minutes != 0 {
			offset += fmt.Sprintf(" %d minutes", -minutes)
		}
		return offset
	}

	if common.UsingPostgreSQL {
		tzName := "UTC"
		if tzEnv := os.Getenv("TZ"); tzEnv != "" {
			tzName = tzEnv
		} else {
			zone, _ := now.Zone()
			if zone != "" {
				tzName = zone
			}
		}
		groupSelect = fmt.Sprintf(`TO_CHAR(date_trunc('%s', to_timestamp(%s) AT TIME ZONE '%s'), '%s') as %s`, groupType, fieldName, tzName, dateFormat, alias)
	} else if common.UsingSQLite {
		groupSelect = fmt.Sprintf(`strftime('%s', datetime(%s, 'unixepoch', '%s')) as %s`, dateFormat, fieldName, getSqliteOffset(), alias)
	} else {
		// MySQL: 检测 MySQL 时区，决定是否需要转换
		var mysqlTz string
		DB.Raw("SELECT @@session.time_zone").Scan(&mysqlTz)
		mysqlIsUTC := mysqlTz == "UTC" || mysqlTz == "+00:00"

		if mysqlIsUTC {
			// MySQL 是 UTC，需要转换为本地时区
			hours := offsetSeconds / 3600
			minutes := (offsetSeconds % 3600) / 60
			var tzOffset string
			if hours >= 0 {
				tzOffset = fmt.Sprintf("+%02d:%02d", hours, minutes)
			} else {
				tzOffset = fmt.Sprintf("-%02d:%02d", -hours, -minutes)
			}
			groupSelect = fmt.Sprintf(`DATE_FORMAT(CONVERT_TZ(FROM_UNIXTIME(%s), '+00:00', '%s'), '%s') as %s`, fieldName, tzOffset, dateFormat, alias)
		} else {
			// MySQL 是本地时区（SYSTEM 或 +08:00 等），直接使用
			groupSelect = fmt.Sprintf(`DATE_FORMAT(FROM_UNIXTIME(%s), '%s') as %s`, fieldName, dateFormat, alias)
		}
	}

	return groupSelect
}

func quotePostgresField(field string) string {
	if common.UsingPostgreSQL {
		return fmt.Sprintf(`"%s"`, field)
	}

	return fmt.Sprintf("`%s`", field)
}

func assembleSumSelectStr(selectStr string) string {
	sumSelectStr := "%s(sum(%s),0)"
	nullfunc := "ifnull"
	if common.UsingPostgreSQL {
		nullfunc = "coalesce"
	}

	sumSelectStr = fmt.Sprintf(sumSelectStr, nullfunc, selectStr)

	return sumSelectStr
}

func RecordExists(table interface{}, fieldName string, fieldValue interface{}, excludeID interface{}) bool {
	var count int64
	query := DB.Model(table).Where(fmt.Sprintf("%s = ?", fieldName), fieldValue)
	if excludeID != nil {
		query = query.Not("id", excludeID)
	}
	query.Count(&count)
	return count > 0
}

// RecordExistsWithTx 在指定事务中检查记录是否存在
func RecordExistsWithTx(tx *gorm.DB, table interface{}, fieldName string, fieldValue interface{}, excludeID interface{}) bool {
	var count int64
	query := tx.Model(table).Where(fmt.Sprintf("%s = ?", fieldName), fieldValue)
	if excludeID != nil {
		query = query.Not("id", excludeID)
	}
	query.Count(&count)
	return count > 0
}

func GetFieldsByID(model interface{}, fieldNames []string, id int, result interface{}) error {
	err := DB.Model(model).Where("id = ?", id).Select(fieldNames).Find(result).Error
	return err
}

func UpdateFieldsByID(model interface{}, id int, fields map[string]interface{}) error {
	err := DB.Model(model).Where("id = ?", id).Updates(fields).Error
	return err
}

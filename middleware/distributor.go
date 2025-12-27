package middleware

import (
	"done-hub/model"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

// GroupDistributor 统一分组分发逻辑
type GroupDistributor struct {
	context *gin.Context
}

// NewGroupDistributor 创建分组分发器
func NewGroupDistributor(c *gin.Context) *GroupDistributor {
	return &GroupDistributor{context: c}
}

// SetupGroups 设置用户分组和令牌分组
func (gd *GroupDistributor) SetupGroups() error {
	userId := gd.context.GetInt("id")
	userGroup, _ := model.CacheGetUserGroup(userId)
	gd.context.Set("group", userGroup)

	tokenGroup := gd.context.GetString("token_group")
	backupGroup := gd.context.GetString("token_backup_group")

	// 验证用户是否有权使用 token_group
	if err := gd.validateGroupPermission(tokenGroup, userGroup); err != nil {
		abortWithMessage(gd.context, http.StatusForbidden, err.Error())
		return err
	}

	// 验证用户是否有权使用 token_backup_group
	if err := gd.validateGroupPermission(backupGroup, userGroup); err != nil {
		abortWithMessage(gd.context, http.StatusForbidden, err.Error())
		return err
	}

	// 初始设置分组比例（使用第一优先级分组）
	// 注意：实际使用的分组可能会在relay层根据渠道可用性动态调整
	initialGroup := gd.getInitialGroup(tokenGroup, backupGroup, userGroup)
	return gd.setGroupRatio(initialGroup)
}

// validateGroupPermission 验证用户是否有权使用指定分组
func (gd *GroupDistributor) validateGroupPermission(group, userGroup string) error {
	if group == "" {
		return nil
	}

	// 如果是用户自己的分组，允许使用
	if group == userGroup {
		return nil
	}

	// 检查分组是否为公开分组
	groupRatio := model.GlobalUserGroupRatio.GetBySymbol(group)
	if groupRatio == nil {
		return fmt.Errorf("分组 %s 不存在", group)
	}

	if !groupRatio.Public {
		return fmt.Errorf("无权使用分组 %s", group)
	}

	return nil
}

// getInitialGroup 获取初始分组（用于设置初始倍率）
func (gd *GroupDistributor) getInitialGroup(tokenGroup, backupGroup, userGroup string) string {
	if tokenGroup != "" {
		return tokenGroup
	}
	if backupGroup != "" {
		return backupGroup
	}
	return userGroup
}

// setGroupRatio 设置分组倍率
func (gd *GroupDistributor) setGroupRatio(group string) error {
	groupRatio := model.GlobalUserGroupRatio.GetBySymbol(group)
	if groupRatio == nil {
		abortWithMessage(gd.context, http.StatusForbidden, fmt.Sprintf("分组 %s 不存在", group))
		return fmt.Errorf("分组 %s 不存在", group)
	}

	gd.context.Set("group_ratio", groupRatio.Ratio)
	return nil
}

func Distribute() func(c *gin.Context) {
	return func(c *gin.Context) {
		distributor := NewGroupDistributor(c)
		if err := distributor.SetupGroups(); err != nil {
			return
		}
		c.Next()
	}
}

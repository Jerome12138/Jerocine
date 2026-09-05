package cache

import (
	"context"
	"encoding/json"
	"time"
)

// 扫码登录(设备码流程, 协调态 db1)。
// deviceCode(长随机, 待登录端轮询用) ↔ userCode(短码, 二维码给手机扫确认)。
//   v1:devauth:dc:<deviceCode> → JSON{userCode,status,userId}
//   v1:devauth:uc:<userCode>   → deviceCode(确认时按短码反查)
const (
	DeviceCodePending    = "pending"
	DeviceCodeAuthorized = "authorized"
)

// DeviceAuth 设备码状态。
type DeviceAuth struct {
	UserCode string `json:"userCode"`
	Status   string `json:"status"`
	UserID   int64  `json:"userId"`
}

func keyDevDC(dc string) string { return "v1:devauth:dc:" + dc }
func keyDevUC(uc string) string { return "v1:devauth:uc:" + uc }

// SaveDeviceCode 写入一对待确认的 deviceCode/userCode(pending)。
func SaveDeviceCode(ctx context.Context, deviceCode, userCode string, ttl time.Duration) error {
	if coordRdb == nil {
		return nil
	}
	b, _ := json.Marshal(DeviceAuth{UserCode: userCode, Status: DeviceCodePending})
	if err := coordRdb.Set(ctx, keyDevDC(deviceCode), b, ttl).Err(); err != nil {
		return err
	}
	return coordRdb.Set(ctx, keyDevUC(userCode), deviceCode, ttl).Err()
}

// GetDeviceCode 取 deviceCode 状态; 不存在/过期返回 (nil, err)。
func GetDeviceCode(ctx context.Context, deviceCode string) (*DeviceAuth, error) {
	if coordRdb == nil {
		return nil, nil
	}
	s, err := coordRdb.Get(ctx, keyDevDC(deviceCode)).Result()
	if err != nil {
		return nil, err
	}
	var da DeviceAuth
	if err := json.Unmarshal([]byte(s), &da); err != nil {
		return nil, err
	}
	return &da, nil
}

// AuthorizeByUserCode 手机端确认: 按 userCode 反查 deviceCode, 置 authorized + 绑定 userId。
// 返回 false 表示 userCode 不存在/已过期。
func AuthorizeByUserCode(ctx context.Context, userCode string, userId int64, ttl time.Duration) (bool, error) {
	if coordRdb == nil {
		return false, nil
	}
	dc, err := coordRdb.Get(ctx, keyDevUC(userCode)).Result()
	if err != nil {
		return false, nil // 不存在/过期
	}
	da, err := GetDeviceCode(ctx, dc)
	if err != nil || da == nil {
		return false, err
	}
	da.Status = DeviceCodeAuthorized
	da.UserID = userId
	b, _ := json.Marshal(da)
	if err := coordRdb.Set(ctx, keyDevDC(dc), b, ttl).Err(); err != nil {
		return false, err
	}
	return true, nil
}

// DeleteDeviceCode 删除一对码(poll 成功后一次性清除)。
func DeleteDeviceCode(ctx context.Context, deviceCode, userCode string) {
	if coordRdb != nil {
		coordRdb.Del(ctx, keyDevDC(deviceCode))
		if userCode != "" {
			coordRdb.Del(ctx, keyDevUC(userCode))
		}
	}
}

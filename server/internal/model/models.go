package model

import "time"

type User struct {
	ID           string    `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey"`
	Account      string    `gorm:"column:account"`
	PasswordHash string    `gorm:"column:password_hash"`
	Nickname     string    `gorm:"column:nickname"`
	AvatarSeed   string    `gorm:"column:avatar_seed"`
	AvatarURL    *string   `gorm:"column:avatar_url"`
	Bio          *string   `gorm:"column:bio"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (User) TableName() string {
	return "users"
}

type MediaTag struct {
	ID         string    `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey"`
	Slug       string    `gorm:"column:slug"`
	Name       string    `gorm:"column:name"`
	IsFeatured bool      `gorm:"column:is_featured"`
	IsActive   bool      `gorm:"column:is_active"`
	SortOrder  int       `gorm:"column:sort_order"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (MediaTag) TableName() string {
	return "media_tags"
}

type MediaSeason struct {
	ID             string    `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey"`
	Slug           string    `gorm:"column:slug"`
	Title          string    `gorm:"column:title"`
	OriginalTitle  *string   `gorm:"column:original_title"`
	Description    *string   `gorm:"column:description"`
	CoverURL       *string   `gorm:"column:cover_url"`
	Category       *string   `gorm:"column:category"`
	ProductionTeam *string   `gorm:"column:production_team"`
	SearchAliases  string    `gorm:"column:search_aliases;type:jsonb"`
	SeasonNumber   *int      `gorm:"column:season_number"`
	SeasonLabel    *string   `gorm:"column:season_label"`
	SortOrder      int       `gorm:"column:sort_order"`
	Status         string    `gorm:"column:status"`
	CreatedAt      time.Time `gorm:"column:created_at"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
}

func (MediaSeason) TableName() string {
	return "media_seasons"
}

type MediaEpisode struct {
	ID            string    `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey"`
	SeasonID      string    `gorm:"column:season_id;type:uuid"`
	Title         string    `gorm:"column:title"`
	Subtitle      *string   `gorm:"column:subtitle"`
	Description   *string   `gorm:"column:description"`
	CoverURL      *string   `gorm:"column:cover_url"`
	MediaURL      string    `gorm:"column:media_url"`
	DurationMs    *int64    `gorm:"column:duration_ms"`
	EpisodeNumber *int      `gorm:"column:episode_number"`
	EpisodeLabel  *string   `gorm:"column:episode_label"`
	SourceKey     string    `gorm:"column:source_key"`
	SourceHash    *string   `gorm:"column:source_hash"`
	SortOrder     int       `gorm:"column:sort_order"`
	Status        string    `gorm:"column:status"`
	CreatedAt     time.Time `gorm:"column:created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

func (MediaEpisode) TableName() string {
	return "media_episodes"
}

type MediaSeasonTag struct {
	SeasonID   string    `gorm:"column:season_id;type:uuid;primaryKey"`
	MediaTagID string    `gorm:"column:media_tag_id;type:uuid;primaryKey"`
	CreatedAt  time.Time `gorm:"column:created_at"`
}

func (MediaSeasonTag) TableName() string {
	return "media_season_tags"
}

type MediaEpisodeVariant struct {
	ID               string    `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey"`
	MediaEpisodeID   string    `gorm:"column:media_episode_id;type:uuid"`
	VariantKey       string    `gorm:"column:variant_key"`
	Label            string    `gorm:"column:label"`
	PlaylistURL      string    `gorm:"column:playlist_url"`
	Width            *int      `gorm:"column:width"`
	Height           *int      `gorm:"column:height"`
	BandwidthBps     *int      `gorm:"column:bandwidth_bps"`
	Codecs           *string   `gorm:"column:codecs"`
	IsDefault        bool      `gorm:"column:is_default"`
	SortOrder        int       `gorm:"column:sort_order"`
	SegmentCount     *int      `gorm:"column:segment_count"`
	AverageSegmentMs *int      `gorm:"column:average_segment_ms"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (MediaEpisodeVariant) TableName() string {
	return "media_episode_variants"
}

type Room struct {
	ID             string     `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey"`
	RoomCode       string     `gorm:"column:room_code"`
	HostUserID     string     `gorm:"column:host_user_id;type:uuid"`
	MediaEpisodeID string     `gorm:"column:media_episode_id;type:uuid"`
	Status         string     `gorm:"column:status"`
	CreatedAt      time.Time  `gorm:"column:created_at"`
	UpdatedAt      time.Time  `gorm:"column:updated_at"`
	LastEmptyAt    *time.Time `gorm:"column:last_empty_at"`
	DestroyAfter   *time.Time `gorm:"column:destroy_after"`
}

func (Room) TableName() string {
	return "rooms"
}

type RoomMember struct {
	ID        string     `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey"`
	RoomID    string     `gorm:"column:room_id;type:uuid"`
	UserID    string     `gorm:"column:user_id;type:uuid"`
	Role      string     `gorm:"column:role"`
	JoinedAt  time.Time  `gorm:"column:joined_at"`
	LeftAt    *time.Time `gorm:"column:left_at"`
	IsActive  bool       `gorm:"column:is_active"`
}

func (RoomMember) TableName() string {
	return "room_members"
}

type UserMediaProgress struct {
	ID                  string    `gorm:"column:id;type:uuid;default:gen_random_uuid();primaryKey"`
	UserID              string    `gorm:"column:user_id;type:uuid"`
	MediaEpisodeID      string    `gorm:"column:media_episode_id;type:uuid"`
	LastPositionSeconds int       `gorm:"column:last_position_seconds"`
	DurationSeconds     int       `gorm:"column:duration_seconds"`
	LastWatchedAt       time.Time `gorm:"column:last_watched_at"`
	Completed           bool      `gorm:"column:completed"`
	CompletionSource    *string   `gorm:"column:completion_source"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (UserMediaProgress) TableName() string {
	return "user_media_progress"
}


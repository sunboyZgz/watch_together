package com.example.watch_together.pages.video

import android.annotation.SuppressLint
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import androidx.compose.ui.zIndex
import com.example.watch_together.ui.theme.Watch_togetherTheme
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext

private val VideoBackground = Color(0xFF111528)
private val VideoCard = Color(0xFF282E49)
private val VideoCardMuted = Color(0xFF303854)
private val VideoPrimary = Color(0xFFFF82C9)
private val VideoTextPrimary = Color(0xFFF9F3FF)
private val VideoTextSecondary = Color(0xC9D8D0E6)
private val VideoOutline = Color(0x22FFFFFF)
private val VideoSearchFill = Color(0x1AFFFFFF)
private val VideoBottomBar = Color(0xF41E243D)
private val DefaultVisibleTagCount = 3
private val ExpandedTagColumnCount = 5

private val VideoGlowA = Brush.radialGradient(
    colors = listOf(Color(0xFF504AA6), Color(0x00504AA6))
)
private val VideoGlowB = Brush.radialGradient(
    colors = listOf(Color(0xFF7D4574), Color(0x007D4574))
)
private val VideoGlowC = Brush.radialGradient(
    colors = listOf(Color(0xFF41637A), Color(0x0041637A))
)

private data class VideoCandidate(
    val id: String,
    val title: String,
    val description: String,
    val tags: List<String>,
    val coverBrush: Brush
)

private data class VideoTagOption(
    val slug: String?,
    val name: String
)

private val sampleVideos = listOf(
    VideoCandidate(
        id = "violet",
        title = "葬送的芙莉莲",
        description = "治愈冒险 · 适合慢慢看",
        tags = listOf("全部", "治愈", "奇幻"),
        coverBrush = Brush.linearGradient(listOf(Color(0xFF6252AA), Color(0xFF463C7E)))
    ),
    VideoCandidate(
        id = "jjk",
        title = "咒术回战",
        description = "热血战斗 · 节奏很顶",
        tags = listOf("全部", "热血", "奇幻"),
        coverBrush = Brush.linearGradient(listOf(Color(0xFF5749A0), Color(0xFF3E396D)))
    ),
    VideoCandidate(
        id = "bocchi",
        title = "孤独摇滚！",
        description = "轻松日常 · 氛围感很好",
        tags = listOf("全部", "校园", "搞笑"),
        coverBrush = Brush.linearGradient(listOf(Color(0xFF6B53B0), Color(0xFF475A8C)))
    ),
    VideoCandidate(
        id = "suzume",
        title = "铃芽之旅",
        description = "剧场版 · 适合一起看",
        tags = listOf("全部", "剧场版", "公路"),
        coverBrush = Brush.linearGradient(listOf(Color(0xFF47728D), Color(0xFF394E78)))
    )
)

@SuppressLint("UnusedBoxWithConstraintsScope")
@Composable
fun VideoSelectionPage(
    onBackClick: () -> Unit,
    onCreateRoomClick: (episodeId: String) -> Unit,
    modifier: Modifier = Modifier,
    enableRemoteLoad: Boolean = true
) {
    val mediaCatalogClient = remember { MediaCatalogClient() }
    var query by rememberSaveable { mutableStateOf("") }
    var selectedTagSlug by rememberSaveable { mutableStateOf<String?>(null) }
    var isMoreTagsExpanded by rememberSaveable { mutableStateOf(false) }
    var selectedVideoId by rememberSaveable { mutableStateOf(sampleVideos.first().id) }
    var featuredTags by remember { mutableStateOf<List<MediaTag>>(emptyList()) }
    var allTags by remember { mutableStateOf<List<MediaTag>>(emptyList()) }
    var mediaItems by remember { mutableStateOf<List<MediaEpisode>>(emptyList()) }
    var isTagsLoading by rememberSaveable { mutableStateOf(false) }
    var isItemsLoading by rememberSaveable { mutableStateOf(false) }
    var catalogError by rememberSaveable { mutableStateOf<String?>(null) }

    LaunchedEffect(enableRemoteLoad) {
        if (!enableRemoteLoad) return@LaunchedEffect

        isTagsLoading = true
        catalogError = null
        runCatching {
            withContext(Dispatchers.IO) {
                mediaCatalogClient.fetchTags()
            }
        }.onSuccess { tags ->
            featuredTags = tags.featuredTags
            allTags = tags.allTags
        }.onFailure { throwable ->
            catalogError = throwable.message ?: "标签加载失败，请稍后重试"
        }
        isTagsLoading = false
    }

    LaunchedEffect(query, selectedTagSlug, enableRemoteLoad) {
        if (!enableRemoteLoad) return@LaunchedEffect

        delay(300)
        isItemsLoading = true
        catalogError = null
        runCatching {
            withContext(Dispatchers.IO) {
                mediaCatalogClient.fetchItems(
                    query = query,
                    tagSlug = selectedTagSlug,
                    limit = 20
                )
            }
        }.onSuccess { page ->
            mediaItems = page.items
            if (page.items.none { it.episodeId == selectedVideoId }) {
                selectedVideoId = page.items.firstOrNull()?.episodeId.orEmpty()
            }
        }.onFailure { throwable ->
            catalogError = throwable.message ?: "影片加载失败，请稍后重试"
        }
        isItemsLoading = false
    }

    val tagOptions = remember(featuredTags, enableRemoteLoad) {
        val source = if (enableRemoteLoad) {
            featuredTags.map { it.toOption() }
        } else {
            fallbackTagOptions().drop(1)
        }
        listOf(VideoTagOption(slug = null, name = "全部")) + source.take(DefaultVisibleTagCount - 1)
    }
    val expandedTagOptions = remember(allTags, featuredTags, enableRemoteLoad) {
        if (enableRemoteLoad) {
            listOf(VideoTagOption(slug = null, name = "全部")) +
                allTags.ifEmpty { featuredTags }.map { it.toOption() }.distinctBy { it.slug }
        } else {
            fallbackTagOptions()
        }
    }
    val selectedTagName = expandedTagOptions.firstOrNull { it.slug == selectedTagSlug }?.name ?: "全部"
    val videos = remember(mediaItems, query, selectedTagSlug, enableRemoteLoad) {
        if (enableRemoteLoad) {
            mediaItems.mapIndexed { index, item -> item.toVideoCandidate(index) }
        } else {
            sampleVideos.filter { video ->
                val queryMatch = query.isBlank() ||
                    video.title.contains(query.trim(), ignoreCase = true) ||
                    video.description.contains(query.trim(), ignoreCase = true)
                val tagMatch = selectedTagSlug == null ||
                    video.tags.contains(selectedTagName)
                queryMatch && tagMatch
            }
        }
    }
    val selectedVideo = videos.firstOrNull { it.id == selectedVideoId }

    BoxWithConstraints(
        modifier = modifier
            .fillMaxSize()
            .background(VideoBackground)
    ) {
        val compactWidth = maxWidth < 380.dp
        val compactHeight = maxHeight < 760.dp
        val pagePadding = if (compactWidth) 18.dp else 28.dp
        val topPadding = if (compactHeight) 18.dp else 30.dp

        VideoSelectionBackdrop()

        Column(
            modifier = Modifier
                .fillMaxSize()
                .verticalScroll(rememberScrollState())
                .padding(horizontal = pagePadding, vertical = topPadding)
                .padding(bottom = 104.dp),
            verticalArrangement = Arrangement.spacedBy(if (compactHeight) 18.dp else 22.dp)
        ) {
            VideoSelectionHeader(
                compactWidth = compactWidth,
                onBackClick = onBackClick
            )

            SearchField(
                query = query,
                onQueryChange = { query = it },
                compactWidth = compactWidth
            )

            TagFilterSection(
                tags = tagOptions,
                expandedTags = expandedTagOptions,
                selectedTagSlug = selectedTagSlug,
                isMoreTagsExpanded = isMoreTagsExpanded,
                compactWidth = compactWidth,
                onTagClick = { tag ->
                    selectedTagSlug = tag.slug
                    isMoreTagsExpanded = false
                },
                onMoreClick = { isMoreTagsExpanded = !isMoreTagsExpanded }
            )

            if (isTagsLoading || isItemsLoading || catalogError != null) {
                CatalogStatusBanner(
                    isLoading = isTagsLoading || isItemsLoading,
                    error = catalogError
                )
            }

            Text(
                text = if (selectedTagSlug == null) "推荐片单" else "$selectedTagName 片单",
                style = if (compactWidth) {
                    androidx.compose.material3.MaterialTheme.typography.headlineSmall
                } else {
                    androidx.compose.material3.MaterialTheme.typography.headlineMedium
                }.copy(
                    color = VideoTextPrimary,
                    fontWeight = FontWeight.Bold
                )
            )

            if (videos.isEmpty()) {
                EmptyResultCard()
            } else {
                VideoGrid(
                    videos = videos,
                    selectedVideoId = selectedVideoId,
                    compactWidth = compactWidth,
                    onVideoClick = { selectedVideoId = it }
                )
            }
        }

        SelectedVideoBottomBar(
            selectedTitle = selectedVideo?.title ?: "未选择",
            enabled = selectedVideo != null,
            compactWidth = compactWidth,
            onCreateRoomClick = {
                selectedVideo?.let { onCreateRoomClick(it.id) }
            },
            modifier = Modifier
                .align(Alignment.BottomCenter)
                .padding(horizontal = pagePadding, vertical = 16.dp)
        )
    }
}

@Composable
private fun VideoSelectionHeader(
    compactWidth: Boolean,
    onBackClick: () -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        Surface(
            modifier = Modifier.clickable(onClick = onBackClick),
            shape = RoundedCornerShape(999.dp),
            color = Color(0x14FFFFFF),
            border = BorderStroke(1.dp, VideoOutline)
        ) {
            Text(
                text = "返回",
                style = androidx.compose.material3.MaterialTheme.typography.labelLarge.copy(
                    color = VideoTextSecondary,
                    fontWeight = FontWeight.SemiBold
                ),
                modifier = Modifier.padding(horizontal = 14.dp, vertical = 8.dp)
            )
        }

        Text(
            text = "挑一部今夜想一起看的",
            style = if (compactWidth) {
                androidx.compose.material3.MaterialTheme.typography.headlineLarge
            } else {
                androidx.compose.material3.MaterialTheme.typography.displaySmall
            }.copy(
                color = VideoTextPrimary,
                fontWeight = FontWeight.Bold
            )
        )
    }
}

@Composable
private fun SearchField(
    query: String,
    onQueryChange: (String) -> Unit,
    compactWidth: Boolean
) {
    OutlinedTextField(
        value = query,
        onValueChange = onQueryChange,
        modifier = Modifier.fillMaxWidth(),
        singleLine = true,
        placeholder = {
            Text(
                text = if (compactWidth) "搜索番剧、剧场版或团队名" else "搜索想一起看的番剧、剧场版或作品名",
                color = VideoTextSecondary
            )
        },
        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
        shape = RoundedCornerShape(26.dp),
        colors = OutlinedTextFieldDefaults.colors(
            focusedContainerColor = VideoSearchFill,
            unfocusedContainerColor = VideoSearchFill,
            focusedBorderColor = Color(0x35FFFFFF),
            unfocusedBorderColor = VideoOutline,
            focusedTextColor = VideoTextPrimary,
            unfocusedTextColor = VideoTextPrimary,
            cursorColor = VideoTextPrimary
        )
    )
}

@Composable
private fun TagFilterSection(
    tags: List<VideoTagOption>,
    expandedTags: List<VideoTagOption>,
    selectedTagSlug: String?,
    isMoreTagsExpanded: Boolean,
    compactWidth: Boolean,
    onTagClick: (VideoTagOption) -> Unit,
    onMoreClick: () -> Unit
) {
    Box(modifier = Modifier.fillMaxWidth()) {
        Column(verticalArrangement = Arrangement.spacedBy(10.dp)) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(if (compactWidth) 8.dp else 10.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                tags.forEach { tag ->
                    TagChip(
                        label = tag.name,
                        selected = selectedTagSlug == tag.slug,
                        compactWidth = compactWidth,
                        onClick = { onTagClick(tag) },
                        modifier = Modifier.weight(1f)
                    )
                }
                MoreTagChip(
                    expanded = isMoreTagsExpanded,
                    compactWidth = compactWidth,
                    onClick = onMoreClick,
                    modifier = Modifier.weight(1f)
                )
            }
        }

        if (isMoreTagsExpanded) {
            MoreTagsPopup(
                tags = expandedTags,
                selectedTagSlug = selectedTagSlug,
                compactWidth = compactWidth,
                onTagClick = onTagClick,
                modifier = Modifier
                    .align(Alignment.TopCenter)
                    .padding(top = if (compactWidth) 54.dp else 58.dp)
                    .zIndex(2f)
            )
        }
    }
}

@Composable
private fun TagChip(
    label: String,
    selected: Boolean,
    compactWidth: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val verticalPadding = if (compactWidth) 10.dp else 11.dp
    Surface(
        modifier = modifier.clickable(onClick = onClick),
        shape = RoundedCornerShape(999.dp),
        color = if (selected) VideoPrimary else Color(0x22FFFFFF),
        border = if (selected) null else BorderStroke(1.dp, VideoOutline)
    ) {
        Text(
            text = label,
            style = androidx.compose.material3.MaterialTheme.typography.labelLarge.copy(
                color = if (selected) Color(0xFF361936) else VideoTextPrimary,
                fontWeight = FontWeight.SemiBold
            ),
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(
                horizontal = if (compactWidth) 10.dp else 14.dp,
                vertical = verticalPadding
            )
        )
    }
}

@Composable
private fun MoreTagChip(
    expanded: Boolean,
    compactWidth: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    val verticalPadding = if (compactWidth) 10.dp else 11.dp
    Surface(
        modifier = modifier.clickable(onClick = onClick),
        shape = RoundedCornerShape(999.dp),
        color = Color(0x19FFFFFF),
        border = BorderStroke(1.dp, VideoOutline)
    ) {
        Text(
            text = if (expanded) "收起" else "更多",
            style = androidx.compose.material3.MaterialTheme.typography.labelLarge.copy(
                color = VideoTextPrimary,
            fontWeight = FontWeight.SemiBold
            ),
            textAlign = TextAlign.Center,
            modifier = Modifier.padding(
                horizontal = if (compactWidth) 10.dp else 14.dp,
                vertical = verticalPadding
            )
        )
    }
}

@Composable
private fun MoreTagsPopup(
    tags: List<VideoTagOption>,
    selectedTagSlug: String?,
    compactWidth: Boolean,
    onTagClick: (VideoTagOption) -> Unit,
    modifier: Modifier = Modifier
) {
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(24.dp),
        color = Color(0xF42A2F4B),
        border = BorderStroke(1.dp, VideoOutline),
        shadowElevation = 24.dp
    ) {
        Column(
            modifier = Modifier.padding(if (compactWidth) 10.dp else 12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Text(
                text = "全部标签",
                style = androidx.compose.material3.MaterialTheme.typography.labelLarge.copy(
                    color = VideoTextSecondary,
                    fontWeight = FontWeight.SemiBold
                )
            )
            tags.chunked(ExpandedTagColumnCount).forEach { rowTags ->
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.spacedBy(if (compactWidth) 6.dp else 8.dp)
                ) {
                    rowTags.forEach { tag ->
                        TagChip(
                            label = tag.name,
                            selected = selectedTagSlug == tag.slug,
                            compactWidth = compactWidth,
                            onClick = { onTagClick(tag) },
                            modifier = Modifier.weight(1f)
                        )
                    }
                    repeat(ExpandedTagColumnCount - rowTags.size) {
                        Spacer(modifier = Modifier.weight(1f))
                    }
                }
            }
        }
    }
}

@Composable
private fun CatalogStatusBanner(
    isLoading: Boolean,
    error: String?
) {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(18.dp),
        color = Color(0x332A86A8),
        border = BorderStroke(1.dp, Color(0x22FFFFFF))
    ) {
        Text(
            text = if (isLoading) {
                "正在加载片单..."
            } else {
                error.orEmpty()
            },
            style = androidx.compose.material3.MaterialTheme.typography.bodyMedium.copy(
                color = VideoTextSecondary,
                fontWeight = FontWeight.SemiBold
            ),
            modifier = Modifier.padding(horizontal = 14.dp, vertical = 10.dp)
        )
    }
}

@Composable
private fun VideoGrid(
    videos: List<VideoCandidate>,
    selectedVideoId: String,
    compactWidth: Boolean,
    onVideoClick: (String) -> Unit
) {
    Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
        videos.chunked(2).forEach { rowVideos ->
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(if (compactWidth) 12.dp else 18.dp)
            ) {
                rowVideos.forEach { video ->
                    VideoCard(
                        video = video,
                        selected = video.id == selectedVideoId,
                        compactWidth = compactWidth,
                        onClick = { onVideoClick(video.id) },
                        modifier = Modifier.weight(1f)
                    )
                }
                if (rowVideos.size == 1) {
                    Spacer(modifier = Modifier.weight(1f))
                }
            }
        }
    }
}

@Composable
private fun VideoCard(
    video: VideoCandidate,
    selected: Boolean,
    compactWidth: Boolean,
    onClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Surface(
        modifier = modifier.clickable(onClick = onClick),
        shape = RoundedCornerShape(26.dp),
        color = if (selected) Color(0xFF343D5D) else VideoCard,
        border = BorderStroke(if (selected) 2.dp else 1.dp, if (selected) VideoPrimary else VideoOutline)
    ) {
        Column(
            modifier = Modifier.padding(if (compactWidth) 12.dp else 14.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            Box(
                modifier = Modifier
                    .fillMaxWidth()
                    .aspectRatio(1.26f)
                    .clip(RoundedCornerShape(20.dp))
                    .background(video.coverBrush)
            )
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text(
                    text = video.title,
                    style = androidx.compose.material3.MaterialTheme.typography.titleLarge.copy(
                        color = VideoTextPrimary,
                        fontWeight = FontWeight.Bold
                    ),
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )
                Text(
                    text = video.description,
                    style = androidx.compose.material3.MaterialTheme.typography.bodyMedium.copy(
                        color = VideoTextSecondary
                    ),
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )
            }
        }
    }
}

@Composable
private fun EmptyResultCard() {
    Surface(
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(26.dp),
        color = VideoCard,
        border = BorderStroke(1.dp, VideoOutline)
    ) {
        Column(
            modifier = Modifier.padding(horizontal = 18.dp, vertical = 22.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Text(
                text = "没有找到合适的影片",
                style = androidx.compose.material3.MaterialTheme.typography.titleLarge.copy(
                    color = VideoTextPrimary,
                    fontWeight = FontWeight.Bold
                )
            )
            Text(
                text = "换个关键词或标签试试看。",
                style = androidx.compose.material3.MaterialTheme.typography.bodyMedium.copy(
                    color = VideoTextSecondary
                )
            )
        }
    }
}

@Composable
private fun SelectedVideoBottomBar(
    selectedTitle: String,
    enabled: Boolean,
    compactWidth: Boolean,
    onCreateRoomClick: () -> Unit,
    modifier: Modifier = Modifier
) {
    Surface(
        modifier = modifier.fillMaxWidth(),
        shape = RoundedCornerShape(28.dp),
        color = VideoBottomBar,
        border = BorderStroke(1.dp, VideoOutline),
        shadowElevation = 22.dp
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 16.dp, vertical = 14.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            Column(
                modifier = Modifier.weight(1f),
                verticalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                Text(
                    text = "已选中",
                    style = androidx.compose.material3.MaterialTheme.typography.labelMedium.copy(
                        color = VideoTextSecondary
                    )
                )
                Text(
                    text = selectedTitle,
                    style = androidx.compose.material3.MaterialTheme.typography.titleMedium.copy(
                        color = VideoTextPrimary,
                        fontWeight = FontWeight.Bold
                    ),
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
            Button(
                onClick = onCreateRoomClick,
                enabled = enabled,
                shape = RoundedCornerShape(999.dp),
                colors = ButtonDefaults.buttonColors(
                    containerColor = VideoPrimary,
                    disabledContainerColor = Color(0x667A6A82),
                    contentColor = Color(0xFF381734),
                    disabledContentColor = Color(0x88FFF8FF)
                )
            ) {
                Text(
                    text = if (compactWidth) "创建" else "创建房间",
                    style = androidx.compose.material3.MaterialTheme.typography.labelLarge.copy(
                        fontWeight = FontWeight.Bold
                    ),
                    modifier = Modifier.padding(horizontal = 6.dp, vertical = 6.dp)
                )
            }
        }
    }
}

@Composable
private fun VideoSelectionBackdrop() {
    Box(modifier = Modifier.fillMaxSize()) {
        Box(
            modifier = Modifier
                .size(260.dp)
                .align(Alignment.TopStart)
                .clip(CircleShape)
                .background(VideoGlowA)
                .alpha(0.32f)
        )
        Box(
            modifier = Modifier
                .size(270.dp)
                .align(Alignment.TopEnd)
                .padding(top = 86.dp)
                .clip(CircleShape)
                .background(VideoGlowB)
                .alpha(0.28f)
        )
        Box(
            modifier = Modifier
                .size(320.dp)
                .align(Alignment.BottomEnd)
                .padding(bottom = 18.dp)
                .clip(CircleShape)
                .background(VideoGlowC)
                .alpha(0.24f)
        )
        repeat(6) { index ->
            Box(
                modifier = Modifier
                    .size(if (index % 2 == 0) 4.dp else 6.dp)
                    .align(
                        when (index) {
                            0 -> Alignment.TopEnd
                            1 -> Alignment.TopCenter
                            2 -> Alignment.CenterEnd
                            3 -> Alignment.CenterStart
                            4 -> Alignment.BottomCenter
                            else -> Alignment.BottomEnd
                        }
                    )
                    .clip(CircleShape)
                    .background(Color(0x80D6CCE5))
            )
        }
    }
}

@Preview(showBackground = true, widthDp = 390, heightDp = 844)
@Composable
private fun VideoSelectionPagePreview() {
    Watch_togetherTheme {
        VideoSelectionPage(
            onBackClick = {},
            onCreateRoomClick = {},
            enableRemoteLoad = false
        )
    }
}

private fun MediaTag.toOption(): VideoTagOption {
    return VideoTagOption(slug = slug, name = name)
}

private fun MediaEpisode.toVideoCandidate(index: Int): VideoCandidate {
    val descriptionText = listOfNotNull(
        subtitle,
        episodeLabel,
        description
    ).firstOrNull { it.isNotBlank() } ?: "适合一起看的片单"

    return VideoCandidate(
        id = episodeId,
        title = title,
        description = descriptionText,
        tags = tags.map { it.name } + "全部",
        coverBrush = coverBrushFor(index)
    )
}

private fun fallbackTagOptions(): List<VideoTagOption> {
    return listOf("全部", "热血", "治愈", "校园", "剧场版", "恋爱", "悬疑", "奇幻", "科幻", "搞笑", "公路", "群像", "百合")
        .map { name ->
            VideoTagOption(
                slug = if (name == "全部") null else name,
                name = name
            )
        }
}

private fun coverBrushFor(index: Int): Brush {
    val palettes = listOf(
        listOf(Color(0xFF6252AA), Color(0xFF463C7E)),
        listOf(Color(0xFF5749A0), Color(0xFF3E396D)),
        listOf(Color(0xFF6B53B0), Color(0xFF475A8C)),
        listOf(Color(0xFF47728D), Color(0xFF394E78)),
        listOf(Color(0xFF72518E), Color(0xFF4B416F))
    )
    return Brush.linearGradient(palettes[index % palettes.size])
}

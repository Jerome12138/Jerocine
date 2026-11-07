package com.jerocine.player.ui;

import android.content.Context;
import android.graphics.Typeface;
import android.view.LayoutInflater;
import android.view.View;
import android.view.ViewGroup;
import android.widget.TextView;

import androidx.annotation.NonNull;
import androidx.recyclerview.widget.RecyclerView;

import com.jerocine.player.R;

import java.util.ArrayList;
import java.util.List;

/**
 * 直播抽屉「频道列表」适配器 — 纯 UI 层(无 Activity 逻辑): 只渲染文案/高亮/焦点动画,
 * 点击经 {@link OnChannelClick} 回调, 切台业务由 LivePlayerActivity 落地.
 *
 * <p>契约(2026-11-07 用户拍板): 行根节点是 item_live_channel.xml 的 channel_row;
 * 当前频道 = 「▶ 」前缀 + jc_accent + bold; 非当前 = 无前缀 + jc_text + normal;
 * 频道**不分组** —— 本适配器只渲染平铺列表, 无分组头/无区段视图.
 *
 * <p>焦点缩放动画照 {@link com.jerocine.player.PlayerDialogHelper}#makeStepperRow 惯例
 * (focus 时 scaleX/Y 1.12, 120ms), 在 onCreateViewHolder 挂一次, VH 复用期不重挂.
 *
 * <p>取色走 {@link JcUi#col}(编程式 View 的唯一取色入口), 不散写 hex.
 *
 * <p>实现约束: player-core 未直接声明 recyclerview 依赖, androidx.recyclerview 由
 * material 1.12.0 传递提供(解析到 1.0.0; 打包时与 tvshell 显式的 1.3.2 合并) ——
 * 因此这里只用 1.0.0 就有的 API(getAdapterPosition), 不用 1.2+ 的 getBindingAdapterPosition.
 */
public class LiveChannelAdapter extends RecyclerView.Adapter<LiveChannelAdapter.ChannelViewHolder> {

    /** 频道行点击回调 — position 即 titles 下标 */
    public interface OnChannelClick {
        void onChannelClick(int position);
    }

    private final LayoutInflater inflater;
    private final Context context;
    /** 频道文案 — 构造时深拷贝, 之后只经 {@link #setTitles} 整体替换 */
    private final List<String> titles = new ArrayList<>();
    /** 当前播放的频道下标 — 高亮(▶/青色/加粗)的唯一依据 */
    private int currentIndex;
    private final OnChannelClick listener;

    public LiveChannelAdapter(Context context, List<String> titles, OnChannelClick listener) {
        this.inflater = LayoutInflater.from(context);
        this.context = context;
        this.listener = listener;
        setTitles(titles);
    }

    /** 整体替换频道列表并整表刷新; null 视为清空. 越界的 currentIndex 不钳位
     * (高亮暂不显示即可), 由 Activity 在替换后调 {@link #setCurrentIndex} 恢复. */
    public void setTitles(List<String> newTitles) {
        titles.clear();
        if (newTitles != null) {
            titles.addAll(newTitles);
        }
        notifyDataSetChanged();
    }

    /** 刷新当前频道高亮 — 只 notify 新旧两行, 不整表刷(避免长列表滚动跳变). */
    public void setCurrentIndex(int index) {
        if (index == currentIndex) {
            return;
        }
        int old = currentIndex;
        currentIndex = index;
        if (old >= 0 && old < titles.size()) {
            notifyItemChanged(old);
        }
        if (index >= 0 && index < titles.size()) {
            notifyItemChanged(index);
        }
    }

    @NonNull
    @Override
    public ChannelViewHolder onCreateViewHolder(@NonNull ViewGroup parent, int viewType) {
        View row = inflater.inflate(R.layout.item_live_channel, parent, false);
        // 焦点缩放照 makeStepperRow: focus 时 scaleX/Y 1.12 / 120ms
        row.setOnFocusChangeListener((v, hasFocus) ->
                v.animate().scaleX(hasFocus ? 1.12f : 1f).scaleY(hasFocus ? 1.12f : 1f)
                        .setDuration(120).start());
        return new ChannelViewHolder(row);
    }

    @Override
    public void onBindViewHolder(@NonNull ChannelViewHolder holder, int position) {
        String title = titles.get(position);
        boolean current = position == currentIndex;
        // 当前频道: 「▶ 」前缀 + jc_accent + bold; 非当前: 无前缀 + jc_text + normal
        holder.row.setText(current ? "▶ " + title : title);
        holder.row.setTextColor(JcUi.col(context,
                current ? R.color.jc_accent : R.color.jc_text));
        holder.row.setTypeface(null, current ? Typeface.BOLD : Typeface.NORMAL);
        // 点击取实时位置(防 notify 后 position 漂移), NO_POSITION 静默忽略
        holder.row.setOnClickListener(v -> {
            int pos = holder.getAdapterPosition();
            if (pos != RecyclerView.NO_POSITION && listener != null) {
                listener.onChannelClick(pos);
            }
        });
    }

    @Override
    public int getItemCount() {
        return titles.size();
    }

    /** 行 VH — item 根即 channel_row 本体, findViewById 拿回同一实例 */
    static final class ChannelViewHolder extends RecyclerView.ViewHolder {
        final TextView row;

        ChannelViewHolder(@NonNull View itemView) {
            super(itemView);
            row = itemView.findViewById(R.id.channel_row);
        }
    }
}

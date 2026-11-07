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
 * 直播抽屉「源 chips」适配器 — 纯 UI 层(无 Activity 逻辑), 与 {@link LiveChannelAdapter}
 * 同构(数据 + currentIndex + 点击回调 + 焦点缩放): 只渲染文案/高亮/焦点动画,
 * 点击经 {@link OnSourceClick} 回调, 换源业务由 LivePlayerActivity 落地
 * (抽屉点 chip 是辅助路径, 主路径是 ←→ 双击换源, 2026-11-07 用户拍板).
 *
 * <p>契约: chip 根节点是 item_live_source_chip.xml 的 source_chip, 横向 RecyclerView 的 item;
 * 当前源 = 「▶ 」前缀 + jc_accent + bold; 非当前 = 无前缀 + jc_text + normal.
 *
 * <p>焦点缩放动画照 {@link com.jerocine.player.PlayerDialogHelper}#makeStepperRow 惯例
 * (focus 时 scaleX/Y 1.12, 120ms), 在 onCreateViewHolder 挂一次, VH 复用期不重挂.
 *
 * <p>实现约束: player-core 未直接声明 recyclerview 依赖, androidx.recyclerview 由
 * material 1.12.0 传递提供(解析到 1.0.0; 打包时与 tvshell 显式的 1.3.2 合并) ——
 * 因此这里只用 1.0.0 就有的 API(getAdapterPosition), 不用 1.2+ 的 getBindingAdapterPosition.
 */
public class LiveSourceChipAdapter extends RecyclerView.Adapter<LiveSourceChipAdapter.ChipViewHolder> {

    /** 源 chip 点击回调 — position 即 names 下标 */
    public interface OnSourceClick {
        void onSourceClick(int position);
    }

    private final LayoutInflater inflater;
    private final Context context;
    /** 源名称 — 构造时深拷贝, 之后只经 {@link #setNames} 整体替换 */
    private final List<String> names = new ArrayList<>();
    /** 当前播放的源下标 — 高亮(▶/青色/加粗)的唯一依据 */
    private int currentIndex;
    private final OnSourceClick listener;

    public LiveSourceChipAdapter(Context context, List<String> names, OnSourceClick listener) {
        this.inflater = LayoutInflater.from(context);
        this.context = context;
        this.listener = listener;
        setNames(names);
    }

    /** 整体替换源名称列表并整表刷新; null 视为清空. 越界的 currentIndex 不钳位
     * (高亮暂不显示即可), 由 Activity 在替换后调 {@link #setCurrentIndex} 恢复. */
    public void setNames(List<String> newNames) {
        names.clear();
        if (newNames != null) {
            names.addAll(newNames);
        }
        notifyDataSetChanged();
    }

    /** 刷新当前源高亮 — 只 notify 新旧两枚 chip, 不整表刷. */
    public void setCurrentIndex(int index) {
        if (index == currentIndex) {
            return;
        }
        int old = currentIndex;
        currentIndex = index;
        if (old >= 0 && old < names.size()) {
            notifyItemChanged(old);
        }
        if (index >= 0 && index < names.size()) {
            notifyItemChanged(index);
        }
    }

    @NonNull
    @Override
    public ChipViewHolder onCreateViewHolder(@NonNull ViewGroup parent, int viewType) {
        View chip = inflater.inflate(R.layout.item_live_source_chip, parent, false);
        // 焦点缩放照 makeStepperRow: focus 时 scaleX/Y 1.12 / 120ms
        chip.setOnFocusChangeListener((v, hasFocus) ->
                v.animate().scaleX(hasFocus ? 1.12f : 1f).scaleY(hasFocus ? 1.12f : 1f)
                        .setDuration(120).start());
        return new ChipViewHolder(chip);
    }

    @Override
    public void onBindViewHolder(@NonNull ChipViewHolder holder, int position) {
        String name = names.get(position);
        boolean current = position == currentIndex;
        // 当前源: 「▶ 」前缀 + jc_accent + bold; 非当前: 无前缀 + jc_text + normal
        holder.chip.setText(current ? "▶ " + name : name);
        holder.chip.setTextColor(JcUi.col(context,
                current ? R.color.jc_accent : R.color.jc_text));
        holder.chip.setTypeface(null, current ? Typeface.BOLD : Typeface.NORMAL);
        // 点击取实时位置(防 notify 后 position 漂移), NO_POSITION 静默忽略
        holder.chip.setOnClickListener(v -> {
            int pos = holder.getAdapterPosition();
            if (pos != RecyclerView.NO_POSITION && listener != null) {
                listener.onSourceClick(pos);
            }
        });
    }

    @Override
    public int getItemCount() {
        return names.size();
    }

    /** chip VH — item 根即 source_chip 本体, findViewById 拿回同一实例 */
    static final class ChipViewHolder extends RecyclerView.ViewHolder {
        final TextView chip;

        ChipViewHolder(@NonNull View itemView) {
            super(itemView);
            chip = itemView.findViewById(R.id.source_chip);
        }
    }
}

package com.jerocine.player.ui;

import android.content.Context;
import android.view.LayoutInflater;
import android.view.View;
import android.view.ViewGroup;
import android.widget.BaseAdapter;
import android.widget.ImageView;
import android.widget.TextView;

import com.jerocine.player.R;

import java.util.List;

/**
 * JcDialog 统一列表行适配器 — 播放器全部弹窗列表的<b>唯一</b>行实现
 * (取代 framework 内部行与 appcompat include 行两套并存的旧机制, 字体/圆圈/行高不再漂移)。
 *
 * <p>行视觉 token 只定义在 jc_dialog_row.xml(14sp 主文字 / 12sp tag / 32dp 行高 / 16dp 圆圈),
 * 本类不做任何字号尺寸散写; 圆圈用 jc_dialog_radio_on/off 两态图直接切换,
 * 选中态显式绑定 checkedIndex(静态绘制 — 弹窗列表存续期内容不变、点击即 dismiss)。
 */
final class JcDialogRowAdapter extends BaseAdapter {

    private final LayoutInflater inflater;
    private final List<JcDialogItem> items;
    private final boolean checkable;
    private final int checkedIndex;

    JcDialogRowAdapter(Context c, List<JcDialogItem> items, boolean checkable, int checkedIndex) {
        this.inflater = LayoutInflater.from(c);
        this.items = items;
        this.checkable = checkable;
        this.checkedIndex = checkedIndex;
    }

    @Override
    public int getCount() {
        return items.size();
    }

    @Override
    public JcDialogItem getItem(int position) {
        return items.get(position);
    }

    @Override
    public long getItemId(int position) {
        return position;
    }

    @Override
    public View getView(int position, View convertView, ViewGroup parent) {
        View row = convertView != null
                ? convertView
                : inflater.inflate(R.layout.jc_dialog_row, parent, false);
        JcDialogItem item = items.get(position);

        ImageView radio = row.findViewById(R.id.jc_row_radio);
        if (checkable) {
            radio.setVisibility(View.VISIBLE);
            radio.setImageResource(position == checkedIndex
                    ? R.drawable.jc_dialog_radio_on : R.drawable.jc_dialog_radio_off);
        } else {
            radio.setVisibility(View.GONE);
        }

        TextView text = row.findViewById(R.id.jc_row_text);
        text.setText(item.text);

        TextView tag = row.findViewById(R.id.jc_row_tag);
        if (item.tag != null && !item.tag.isEmpty()) {
            tag.setText(item.tag);
            tag.setVisibility(View.VISIBLE);
        } else {
            tag.setVisibility(View.GONE);
        }

        // 无障碍: 整行朗读 主文字+tag
        row.setContentDescription(item.tag == null
                ? item.text : item.text + "，" + item.tag);
        return row;
    }
}

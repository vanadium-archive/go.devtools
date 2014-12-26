package io.v.jenkins.plugins.vanadium_scm;

import hudson.Extension;
import hudson.MarkupText;
import hudson.MarkupText.SubText;
import hudson.model.AbstractBuild;
import hudson.scm.ChangeLogAnnotator;
import hudson.scm.ChangeLogSet.Entry;

import java.util.regex.Pattern;

/**
 * Turns "Change-ID: XXXX" into a hyperlink to Gerrit.
 */
@Extension
public class ChangeIdAnnotator extends ChangeLogAnnotator {
  private static final Pattern CHANGE_ID =
      Pattern.compile("(?<=\\bChange-Id: )I[0-9a-fA-F]{40}\\b");

  private static final String VANADIUM_GERRIT_URL = "https://vanadium-review.googlesource.com/";

  @Override
  public void annotate(AbstractBuild<?, ?> build, Entry change, MarkupText text) {
    for (SubText token : text.findTokens(CHANGE_ID)) {
      token.addMarkup(0, token.length(),
          "<a href='" + (VANADIUM_GERRIT_URL + "r/" + token.getText()) + "' target='_blank'>",
          "</a>");
    }
  }
}
